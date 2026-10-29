package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func retryWaitText(seconds int64) string {
	if seconds < 1 {
		seconds = 1
	}
	parts := []string{}
	for _, unit := range []struct {
		size  int64
		label string
	}{{86400, "天"}, {3600, "小时"}, {60, "分钟"}, {1, "秒"}} {
		if n := seconds / unit.size; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, unit.label))
			seconds %= unit.size
		}
	}
	return strings.Join(parts, " ")
}

func passwordLockedResponse(w http.ResponseWriter, until time.Time) {
	d := time.Until(until)
	seconds := int64(d / time.Second)
	if d%time.Second > 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", fmt.Sprint(seconds))
	fail(w, 429, fmt.Sprintf("账号密码登录已锁定，请在 %s 后重试，或通过邮箱重置密码。", retryWaitText(seconds)))
}

// Saturate only at time.Duration's representable limit instead of overflowing.
func passwordLockDuration(level int) time.Duration {
	d := 10 * time.Minute
	const max = time.Duration(1<<63 - 1)
	for i := 0; i < level; i++ {
		if d > max/6 {
			return max
		}
		d *= 6
	}
	return d
}

// Row locking serializes attempts, including parallel requests from different IPs.
func (s *Server) verifyAccountPassword(ctx context.Context, email, password string) (bool, time.Time, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, time.Time{}, err
	}
	defer tx.Rollback(ctx)
	var id, hash string
	var enabled bool
	var failures, level int
	var until *time.Time
	err = tx.QueryRow(ctx, `SELECT id,password_hash,enabled,password_failures,password_lock_level,password_locked_until FROM users WHERE email=$1 FOR UPDATE`, email).Scan(&id, &hash, &enabled, &failures, &level, &until)
	if errors.Is(err, pgx.ErrNoRows) {
		checkPassword(password, "")
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}
	now := time.Now()
	if until != nil && until.After(now) {
		return false, *until, nil
	}
	valid := checkPassword(password, hash) && enabled
	var locked time.Time
	if valid {
		_, err = tx.Exec(ctx, `UPDATE users SET password_failures=0,password_lock_level=0,password_locked_until=NULL WHERE id=$1`, id)
	} else {
		failures++
		if failures == 5 {
			locked = now.Add(passwordLockDuration(level))
			// The duration already saturates at this level; avoid an integer overflow.
			if level < 32 {
				level++
			}
			_, err = tx.Exec(ctx, `UPDATE users SET password_failures=0,password_lock_level=$2,password_locked_until=$3 WHERE id=$1`, id, level, locked)
		} else {
			_, err = tx.Exec(ctx, `UPDATE users SET password_failures=$2,password_locked_until=NULL WHERE id=$1`, id, failures)
		}
	}
	if err != nil {
		return false, time.Time{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, time.Time{}, err
	}
	return valid, locked, nil
}
