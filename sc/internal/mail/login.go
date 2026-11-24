package mail

import (
	"context"
	"errors"
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
)

// Login is how to sign in to a mailbox: a password (a Google app password), or a function that
// returns a fresh access token from Google's own sign-in. Exactly one of Password and Token is
// set. The token is asked for at each connection, so a long sync never starts with one about to
// expire.
type Login struct {
	Username string
	Password string
	Token    func(context.Context) (string, error)
}

// PasswordLogin signs in with an address and a password.
func PasswordLogin(username, password string) Login {
	return Login{Username: username, Password: password}
}

// TokenError means the sign-in token could not be had or was refused: the user has to sign in
// with Google again. The server's or Google's own words are kept.
type TokenError struct {
	Text string
}

func (e *TokenError) Error() string {
	return "Google sign-in did not work: " + e.Text
}

// authenticate signs in on an open connection.
func authenticate(ctx context.Context, c *imapclient.Client, l Login) error {
	if l.Token == nil {
		err := c.Login(l.Username, l.Password).Wait()
		var refused *imap.Error
		if errors.As(err, &refused) {
			return &LoginError{Text: refused.Text}
		}
		return err
	}

	token, err := l.Token(ctx)
	if err != nil {
		var te *TokenError
		if errors.As(err, &te) {
			return err
		}
		return &TokenError{Text: err.Error()}
	}
	var mech sasl.Client
	caps := c.Caps()
	switch {
	case caps.Has(imap.Cap("AUTH=XOAUTH2")):
		mech = &xoauth2{user: l.Username, token: token}
	case caps.Has(imap.Cap("AUTH=OAUTHBEARER")):
		mech = sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{Username: l.Username, Token: token})
	default:
		return errors.New("this mail server does not offer sign-in with a Google token")
	}
	err = c.Authenticate(mech)
	if err == nil {
		return nil
	}
	var refused *imap.Error
	if errors.As(err, &refused) {
		text := refused.Text
		if x, ok := mech.(*xoauth2); ok && x.challenge != "" {
			text = fmt.Sprintf("%s (%s)", text, x.challenge)
		}
		return &TokenError{Text: text}
	}
	return err
}

// xoauth2 is Google's XOAUTH2 SASL mechanism: the address and the bearer token in one message.
// When the token is refused the server first sends a JSON challenge saying why; the client must
// answer it with an empty response to get the final NO, and the challenge is kept for the error.
type xoauth2 struct {
	user, token string
	challenge   string
}

func (x *xoauth2) Start() (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + x.user + "\x01auth=Bearer " + x.token + "\x01\x01"), nil
}

func (x *xoauth2) Next(challenge []byte) ([]byte, error) {
	x.challenge = string(challenge)
	return []byte{}, nil
}
