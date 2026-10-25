package invite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrInvalidToken = errors.New("token tidak valid, kedaluwarsa, dicabut, atau habis kuota")
	ErrAlreadyRedeemed = errors.New("nomor ini sudah menggunakan token tersebut")
	ErrAlreadyAllowed = errors.New("nomor sudah memiliki akses")
)

type Token struct {
	ID int64
	CreatedAt time.Time
	ExpiresAt time.Time
	Uses int
	MaxUses int
	Revoked bool
}

type Service struct { db *sql.DB }

func Open(dataDir string) (*Service, error) {
	path := filepath.Join(dataDir, "bot.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil { return nil, err }
	db.SetMaxOpenConns(1)
	s := &Service{db: db}
	schema := `
CREATE TABLE IF NOT EXISTS invite_tokens (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 token_hash TEXT NOT NULL UNIQUE,
 created_at INTEGER NOT NULL,
 expires_at INTEGER NOT NULL,
 max_uses INTEGER NOT NULL CHECK(max_uses > 0),
 uses INTEGER NOT NULL DEFAULT 0 CHECK(uses >= 0),
 revoked INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS invite_redemptions (
 token_id INTEGER NOT NULL REFERENCES invite_tokens(id),
 phone TEXT NOT NULL,
 redeemed_at INTEGER NOT NULL,
 PRIMARY KEY(token_id, phone)
);
CREATE TABLE IF NOT EXISTS allowed_users (
 phone TEXT PRIMARY KEY,
 added_at INTEGER NOT NULL,
 source TEXT NOT NULL
);`
	if _, err := db.Exec(schema); err != nil { db.Close(); return nil, err }
	return s, nil
}

func (s *Service) Close() error { return s.db.Close() }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(token))))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Create(ctx context.Context, maxUses int, ttl time.Duration) (string, Token, error) {
	if maxUses < 1 { return "", Token{}, fmt.Errorf("kuota harus minimal 1") }
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil { return "", Token{}, err }
	token := strings.ToUpper(hex.EncodeToString(raw))
	now := time.Now().UTC()
	exp := now.Add(ttl)
	res, err := s.db.ExecContext(ctx, "INSERT INTO invite_tokens(token_hash,created_at,expires_at,max_uses,uses,revoked) VALUES(?,?,?,?,0,0)", hashToken(token), now.Unix(), exp.Unix(), maxUses)
	if err != nil { return "", Token{}, err }
	id, err := res.LastInsertId()
	if err != nil { return "", Token{}, err }
	return token, Token{ID:id, CreatedAt:now, ExpiresAt:exp, MaxUses:maxUses}, nil
}

func (s *Service) List(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,created_at,expires_at,max_uses,uses,revoked FROM invite_tokens ORDER BY id DESC LIMIT 30")
	if err != nil { return nil, err }
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token; var created, expires int64; var revoked int
		if err := rows.Scan(&t.ID,&created,&expires,&t.MaxUses,&t.Uses,&revoked); err != nil { return nil, err }
		t.CreatedAt=time.Unix(created,0).UTC(); t.ExpiresAt=time.Unix(expires,0).UTC(); t.Revoked=revoked != 0
		out=append(out,t)
	}
	return out, rows.Err()
}

func (s *Service) Revoke(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE invite_tokens SET revoked=1 WHERE id=?", id)
	if err != nil { return false, err }
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Service) Redeem(ctx context.Context, token, phone string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" { return ErrInvalidToken }
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	now := time.Now().UTC().Unix()
	var id, expires int64
	var uses, maxUses, revoked int
	err = tx.QueryRowContext(ctx, "SELECT id,expires_at,uses,max_uses,revoked FROM invite_tokens WHERE token_hash=?", hashToken(token)).Scan(&id,&expires,&uses,&maxUses,&revoked)
	if err != nil { return ErrInvalidToken }
	if revoked != 0 || expires <= now || uses >= maxUses { return ErrInvalidToken }
	var count int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM invite_redemptions WHERE token_id=? AND phone=?", id, phone).Scan(&count)
	if err != nil { return err }
	if count > 0 { return ErrAlreadyRedeemed }
	var exists int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM allowed_users WHERE phone=?", phone).Scan(&exists)
	if err != nil { return err }
	if exists > 0 { return ErrAlreadyAllowed }
	if _, err = tx.ExecContext(ctx, "INSERT INTO allowed_users(phone,added_at,source) VALUES(?,?,?)", phone, now, "invite"); err != nil { return err }
	if _, err = tx.ExecContext(ctx, "INSERT INTO invite_redemptions(token_id,phone,redeemed_at) VALUES(?,?,?)", id, phone, now); err != nil { return err }
	res, err := tx.ExecContext(ctx, "UPDATE invite_tokens SET uses=uses+1 WHERE id=? AND revoked=0 AND expires_at>? AND uses<max_uses", id, now)
	if err != nil { return err }
	n, err := res.RowsAffected()
	if err != nil { return err }
	if n != 1 { return ErrInvalidToken }
	return tx.Commit()
}

func (s *Service) AllowedNumbers(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT phone FROM allowed_users ORDER BY phone")
	if err != nil { return nil, err }
	defer rows.Close()
	var numbers []string
	for rows.Next() { var p string; if err := rows.Scan(&p); err != nil { return nil, err }; numbers=append(numbers,p) }
	return numbers, rows.Err()
}
