package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Overview is the shape of one account's inbox, worked out from the copy in DuckDB.
type Overview struct {
	AccountID int64         `json:"accountId"`
	Sync      InboxSync     `json:"sync"`     // whether the copy is complete; the figures below cover only the copy
	Messages  int           `json:"messages"` // in the copy
	Unread    int           `json:"unread"`
	Oldest    *time.Time    `json:"oldest"` // UTC; nil when the copy is empty
	Years     []YearCount   `json:"years"`  // oldest first
	Senders   []SenderCount `json:"senders"`
	// The senders with the most mail whose latest messages say how to unsubscribe.
	Unsubscribable []SenderCount `json:"unsubscribable"`
}

// YearCount is how much mail arrived in one year (UTC), and how much of it is unread.
type YearCount struct {
	Year     int `json:"year"`
	Messages int `json:"messages"`
	Unread   int `json:"unread"`
}

// SenderCount is one sender's mail in the inbox. Address is lower case; Name is the most recent
// display name the sender used.
type SenderCount struct {
	Address  string    `json:"address"`
	Name     string    `json:"name"`
	Messages int       `json:"messages"`
	Unread   int       `json:"unread"`
	Oldest   time.Time `json:"oldest"`
	Latest   time.Time `json:"latest"`
	// How to unsubscribe, from the sender's most recent message that says; nil when none does.
	Unsubscribe *Unsubscribe `json:"unsubscribe"`
}

// topSenders is how many senders the overview lists.
const topSenders = 25

// InboxOverview works out the shape of an account's inbox from its copy.
func (s *Store) InboxOverview(ctx context.Context, accountID int64) (Overview, error) {
	o := Overview{AccountID: accountID, Years: []YearCount{}, Senders: []SenderCount{}, Unsubscribable: []SenderCount{}}
	sync, _, err := s.GetInboxSync(ctx, accountID)
	if err != nil {
		return Overview{}, err
	}
	o.Sync = sync

	var oldest sql.NullTime
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*), count(*) FILTER (WHERE NOT seen), min(received_at) FROM message WHERE account_id = ?`, accountID).
		Scan(&o.Messages, &o.Unread, &oldest); err != nil {
		return Overview{}, fmt.Errorf("overview of account %d: %w", accountID, err)
	}
	if oldest.Valid {
		t := oldest.Time.UTC()
		o.Oldest = &t
	}

	years, err := s.db.QueryContext(ctx,
		`SELECT year(received_at) AS y, count(*), count(*) FILTER (WHERE NOT seen)
		 FROM message WHERE account_id = ? GROUP BY y ORDER BY y`, accountID)
	if err != nil {
		return Overview{}, fmt.Errorf("overview of account %d: %w", accountID, err)
	}
	defer years.Close()
	for years.Next() {
		var y YearCount
		if err := years.Scan(&y.Year, &y.Messages, &y.Unread); err != nil {
			return Overview{}, fmt.Errorf("overview of account %d: %w", accountID, err)
		}
		o.Years = append(o.Years, y)
	}
	if err := years.Err(); err != nil {
		return Overview{}, fmt.Errorf("overview of account %d: %w", accountID, err)
	}

	if o.Senders, err = s.senderCounts(ctx, accountID, false); err != nil {
		return Overview{}, err
	}
	if o.Unsubscribable, err = s.senderCounts(ctx, accountID, true); err != nil {
		return Overview{}, err
	}
	return o, nil
}

// senderCounts lists the senders with the most mail, grouped by lower-case address (the same
// grouping a "sender" action matches on). With onlyUnsubscribable, only senders whose mail says
// how to unsubscribe.
func (s *Store) senderCounts(ctx context.Context, accountID int64, onlyUnsubscribable bool) ([]SenderCount, error) {
	having := ""
	if onlyUnsubscribable {
		having = `HAVING count(unsub_url) + count(unsub_mailto) > 0`
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT coalesce(from_addr, '') AS addr, coalesce(arg_max(from_name, received_at), ''), count(*),
		        count(*) FILTER (WHERE NOT seen), min(received_at), max(received_at),
		        coalesce(arg_max(unsub_url, received_at) FILTER (WHERE unsub_url IS NOT NULL), ''),
		        coalesce(arg_max(unsub_mailto, received_at) FILTER (WHERE unsub_mailto IS NOT NULL), ''),
		        coalesce(arg_max(unsub_one_click, received_at) FILTER (WHERE unsub_url IS NOT NULL), false)
		 FROM message WHERE account_id = ? GROUP BY addr `+having+`
		 ORDER BY count(*) DESC, addr LIMIT ?`, accountID, topSenders)
	if err != nil {
		return nil, fmt.Errorf("senders of account %d: %w", accountID, err)
	}
	defer rows.Close()
	out := []SenderCount{}
	for rows.Next() {
		var c SenderCount
		var u Unsubscribe
		if err := rows.Scan(&c.Address, &c.Name, &c.Messages, &c.Unread, &c.Oldest, &c.Latest, &u.URL, &u.Mailto, &u.OneClick); err != nil {
			return nil, fmt.Errorf("senders of account %d: %w", accountID, err)
		}
		c.Oldest, c.Latest = c.Oldest.UTC(), c.Latest.UTC()
		if u.URL != "" || u.Mailto != "" {
			c.Unsubscribe = &u
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SenderUnsubscribe returns how a sender's most recent mail says to unsubscribe. It is empty when
// none of the sender's mail says.
func (s *Store) SenderUnsubscribe(ctx context.Context, accountID int64, address string) (Unsubscribe, error) {
	var u Unsubscribe
	err := s.db.QueryRowContext(ctx,
		`SELECT coalesce(arg_max(unsub_url, received_at) FILTER (WHERE unsub_url IS NOT NULL), ''),
		        coalesce(arg_max(unsub_mailto, received_at) FILTER (WHERE unsub_mailto IS NOT NULL), ''),
		        coalesce(arg_max(unsub_one_click, received_at) FILTER (WHERE unsub_url IS NOT NULL), false)
		 FROM message WHERE account_id = ? AND coalesce(from_addr, '') = ?`,
		accountID, strings.ToLower(strings.TrimSpace(address))).Scan(&u.URL, &u.Mailto, &u.OneClick)
	if err != nil {
		return Unsubscribe{}, fmt.Errorf("unsubscribe address of %s: %w", address, err)
	}
	return u, nil
}
