package store

import (
	"context"
	"fmt"
	"strings"
)

// SearchLimit is the most messages a search returns.
const SearchLimit = 200

// SearchMessages finds messages in the copy of the inbox whose sender (name or address) or subject
// has every word of the query, newest first, in any account or in one (accountID > 0). Only
// headers are kept, so only headers are searched. more says there were more than SearchLimit.
func (s *Store) SearchMessages(ctx context.Context, query string, accountID int64) (msgs []Message, more bool, err error) {
	words := strings.Fields(strings.ToLower(query))
	if len(words) > 8 {
		words = words[:8]
	}
	msgs = []Message{}
	if len(words) == 0 {
		return msgs, false, nil
	}
	conds := make([]string, 0, len(words)+1)
	args := make([]any, 0, len(words)*3+2)
	for _, w := range words {
		like := "%" + escapeLike(w) + "%"
		conds = append(conds, `(coalesce(from_name, '') ILIKE ? ESCAPE '\' OR coalesce(from_addr, '') ILIKE ? ESCAPE '\'
		                        OR coalesce(subject, '') ILIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	if accountID > 0 {
		conds = append(conds, "account_id = ?")
		args = append(args, accountID)
	}
	args = append(args, SearchLimit+1)
	rows, err := s.db.QueryContext(ctx,
		`SELECT account_id, uid_validity, uid, coalesce(message_id, ''), coalesce(from_name, ''), coalesce(from_addr, ''),
		        coalesce(subject, ''), received_at, seen
		 FROM message WHERE `+strings.Join(conds, " AND ")+`
		 ORDER BY received_at DESC, account_id, uid DESC LIMIT ?`, args...)
	if err != nil {
		return nil, false, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m Message
		var validity, uid int64
		if err := rows.Scan(&m.AccountID, &validity, &uid, &m.MessageID, &m.FromName, &m.FromAddr,
			&m.Subject, &m.ReceivedAt, &m.Seen); err != nil {
			return nil, false, fmt.Errorf("search messages: %w", err)
		}
		m.UIDValidity, m.UID = uint32(validity), uint32(uid)
		m.ReceivedAt = m.ReceivedAt.UTC()
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(msgs) > SearchLimit {
		return msgs[:SearchLimit], true, nil
	}
	return msgs, false, nil
}

// escapeLike makes % and _ in a search word match themselves.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
