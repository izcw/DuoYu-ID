package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

type queuedMail struct {
	Message                         MailMessage
	Email, Purpose, ChallengeDigest string
}
type deliveryError struct {
	kind  string
	cause error
}

func (e *deliveryError) Error() string { return "mail transport: " + e.kind }
func (e *deliveryError) Unwrap() error { return e.cause }

func (s *Server) mailCipher() (cipher.AEAD, error) {
	key := sha256.Sum256(append([]byte("duoyu-mail-payload-v1:"), s.key[:]...))
	block, e := aes.NewCipher(key[:])
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(block)
}
func (s *Server) encryptMail(id string, m queuedMail) ([]byte, error) {
	a, e := s.mailCipher()
	if e != nil {
		return nil, e
	}
	body, e := json.Marshal(m)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return a.Seal(nonce, nonce, body, []byte(id)), nil
}
func (s *Server) decryptMail(id string, body []byte) (queuedMail, error) {
	var out queuedMail
	a, e := s.mailCipher()
	if e != nil {
		return out, e
	}
	if len(body) < a.NonceSize() {
		return out, errors.New("invalid mail payload")
	}
	plain, e := a.Open(nil, body[:a.NonceSize()], body[a.NonceSize():], []byte(id))
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(plain, &out)
	return out, e
}

func (s *Server) enqueueAuth(m MailMessage, email, purpose, code string) error {
	id := "auth_" + randomID()
	payload, e := s.encryptMail(id, queuedMail{Message: m, Email: email, Purpose: purpose, ChallengeDigest: s.mac(email + "|" + purpose + "|" + code)})
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Use the challenge's actual expiry, not five minutes after queue insertion.
	tag, e := s.db.Exec(ctx, `INSERT INTO mail_jobs(id,lane,payload,expires_at) SELECT $1,'auth',$2,expires_at FROM challenges WHERE email=$3 AND purpose=$4 AND digest=$5 AND expires_at>now()`, id, payload, email, purpose, s.mac(email+"|"+purpose+"|"+code))
	if e == nil && tag.RowsAffected() != 1 {
		return errors.New("challenge no longer valid")
	}
	return e
}

func (s *Server) sweepMail(ctx context.Context) error {
	_, e := s.db.Exec(ctx, `WITH changed AS (UPDATE mail_jobs SET status=CASE WHEN status='sending' THEN 'unknown' ELSE 'expired' END,payload=NULL,error_code=CASE WHEN status='sending' THEN 'worker_interrupted' ELSE 'expired' END WHERE (status='sending' AND locked_at<now()-interval '2 minutes') OR (status IN ('queued','retry') AND expires_at<=now()) RETURNING message_id,status,error_code) UPDATE mail_messages m SET status=c.status,error_code=c.error_code,updated_at=now() FROM changed c WHERE m.id=c.message_id`)
	return e
}

func (s *Server) processMail(ctx context.Context, lane string) (bool, error) {
	if e := s.sweepMail(ctx); e != nil {
		return false, e
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var id string
	var mid *string
	var payload []byte
	var attempts int
	e = tx.QueryRow(ctx, `SELECT id,message_id,payload,attempts FROM mail_jobs WHERE lane=$1 AND status IN ('queued','retry') AND available_at<=now() AND expires_at>now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`, lane).Scan(&id, &mid, &payload, &attempts)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if _, e = tx.Exec(ctx, "UPDATE mail_jobs SET status='sending',locked_at=now(),attempts=attempts+1 WHERE id=$1", id); e != nil {
		return false, e
	}
	if mid != nil {
		if _, e = tx.Exec(ctx, "UPDATE mail_messages SET status='sending',updated_at=now() WHERE id=$1", *mid); e != nil {
			return false, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return false, e
	}
	status, reason := "accepted", ""
	m, err := s.decryptMail(id, payload)
	if err != nil {
		status, reason = "failed", "payload_unavailable"
	} else {
		valid := true
		if lane == "auth" {
			err = s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM challenges WHERE email=$1 AND purpose=$2 AND digest=$3 AND expires_at>now())", m.Email, m.Purpose, m.ChallengeDigest).Scan(&valid)
		} else if mid != nil {
			err = s.db.QueryRow(ctx, `SELECT a.enabled AND NOT k.revoked AND (k.expires_at IS NULL OR k.expires_at>now()) AND 'mail:send'=ANY(k.scopes) FROM mail_messages m JOIN apps a ON a.id=m.app_id JOIN api_keys k ON k.id=m.key_id WHERE m.id=$1`, *mid).Scan(&valid)
		}
		if err != nil {
			status, reason = "retry", "validation_unavailable"
		} else if !valid {
			status, reason = "cancelled", "authorization_or_challenge_expired"
		} else {
			err = s.sendMessage(m.Message)
			if err != nil {
				var transport *deliveryError
				if errors.As(err, &transport) {
					status = transport.kind
				} else {
					status = "unknown"
				}
				reason = "smtp_" + status
			}
		}
	}
	if status == "retry" && attempts+1 >= 3 {
		status, reason = "failed", "retry_exhausted"
	}
	// Persist outcome even when the HTTP process is being shut down.
	done, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finish, e := s.db.Begin(done)
	if e != nil {
		return true, e
	}
	defer finish.Rollback(done)
	delay := time.Duration(30*(attempts+1)) * time.Second
	tag, e := finish.Exec(done, `UPDATE mail_jobs SET status=$2,error_code=$3,payload=CASE WHEN $2='retry' THEN payload ELSE NULL END,available_at=$4 WHERE id=$1 AND status='sending'`, id, status, reason, time.Now().Add(delay))
	if e != nil {
		return true, e
	}
	if tag.RowsAffected() == 1 && mid != nil {
		if _, e = finish.Exec(done, "UPDATE mail_messages SET status=$2,error_code=$3,updated_at=now() WHERE id=$1", *mid, status, reason); e != nil {
			return true, e
		}
	}
	if e = finish.Commit(done); e != nil {
		return true, e
	}
	return true, nil
}

func (s *Server) startMailWorkers(ctx context.Context, wg *sync.WaitGroup) {
	for _, lane := range []string{"auth", "project"} {
		wg.Add(1)
		go func(lane string) {
			defer wg.Done()
			timer := time.NewTicker(time.Second)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					if _, e := s.processMail(ctx, lane); e != nil && ctx.Err() == nil {
						slog.Error("mail worker operation failed", "lane", lane)
					}
				}
			}
		}(lane)
	}
}
