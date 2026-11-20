package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/xiaotuantuan5/blog/internal/db"
)

const SessionTTL = 30 * 24 * time.Hour

var ErrBadCredentials = errors.New("用户名或密码错误")

// Session is an authenticated login.
type Session struct {
	AdminID   int64
	Username  string
	CSRFToken string
	ExpiresAt time.Time
}

func validatePassword(pw string) error {
	if utf8.RuneCountInString(pw) < 8 {
		return invalid("密码至少 8 位")
	}
	return nil
}

// AdminExists reports whether the admin account has been created.
func (a *App) AdminExists(ctx context.Context) (bool, error) {
	var n int
	err := a.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin`).Scan(&n)
	return n > 0, err
}

// CreateAdmin creates the single admin account.
func (a *App) CreateAdmin(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return invalid("用户名不能为空")
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	exists, err := a.AdminExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return conflict("管理员已存在，如需修改密码请使用 reset-password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = a.DB.ExecContext(ctx,
		`INSERT INTO admin (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, string(hash), db.Now())
	return err
}

// ResetPassword sets a new password and signs out every session.
func (a *App) ResetPassword(ctx context.Context, username, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return a.inTx(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM admin WHERE username = ?`, username).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE admin SET password_hash = ? WHERE id = ?`, string(hash), id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE admin_id = ?`, id)
		return err
	})
}

// ChangePassword verifies the old password, sets the new one and signs out
// other sessions.
func (a *App) ChangePassword(ctx context.Context, s *Session, oldPw, newPw, keepToken string) error {
	var hash string
	if err := a.DB.QueryRowContext(ctx, `SELECT password_hash FROM admin WHERE id = ?`, s.AdminID).Scan(&hash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(oldPw)) != nil {
		return invalid("原密码错误")
	}
	if err := validatePassword(newPw); err != nil {
		return err
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return a.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE admin SET password_hash = ? WHERE id = ?`, string(newHash), s.AdminID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE admin_id = ? AND token_hash <> ?`, s.AdminID, hashToken(keepToken))
		return err
	})
}

// dummyHash keeps login timing similar when the username doesn't exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

// Login checks credentials and creates a session. It returns the raw
// session token, which is only ever stored hashed.
func (a *App) Login(ctx context.Context, username, password string) (string, *Session, error) {
	var (
		id   int64
		hash string
	)
	err := a.DB.QueryRowContext(ctx, `SELECT id, password_hash FROM admin WHERE username = ?`, username).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", nil, ErrBadCredentials
	} else if err != nil {
		return "", nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", nil, ErrBadCredentials
	}

	token := randomToken()
	s := &Session{
		AdminID:   id,
		Username:  username,
		CSRFToken: randomToken(),
		ExpiresAt: time.Now().Add(SessionTTL),
	}
	now := db.Now()
	if _, err := a.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now); err != nil {
		return "", nil, err
	}
	_, err = a.DB.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, admin_id, csrf_token, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		hashToken(token), id, s.CSRFToken, db.Time{Time: s.ExpiresAt}, now)
	if err != nil {
		return "", nil, err
	}
	return token, s, nil
}

// SessionByToken returns the session for a raw token, or ErrNotFound.
func (a *App) SessionByToken(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	var (
		s       Session
		expires db.Time
	)
	err := a.DB.QueryRowContext(ctx, `
		SELECT s.admin_id, a.username, s.csrf_token, s.expires_at
		FROM sessions s JOIN admin a ON a.id = s.admin_id
		WHERE s.token_hash = ?`, hashToken(token)).Scan(&s.AdminID, &s.Username, &s.CSRFToken, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if time.Now().After(expires.Time) {
		_, _ = a.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
		return nil, ErrNotFound
	}
	s.ExpiresAt = expires.Time
	return &s, nil
}

func (a *App) Logout(ctx context.Context, token string) error {
	_, err := a.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

// CheckCSRF compares tokens in constant time.
func (s *Session) CheckCSRF(token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.CSRFToken)) == 1
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}
