package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultRedisPrimaryFailureKeyPrefix = "platform:official:primary_failure:"

type RedisOfficialPrimaryFailureStoreConfig struct {
	URL         string
	KeyPrefix   string
	DialTimeout time.Duration
	IOTimeout   time.Duration
}

type redisOfficialPrimaryFailureStore struct {
	addr        string
	username    string
	password    string
	db          int
	keyPrefix   string
	dialTimeout time.Duration
	ioTimeout   time.Duration
}

func NewRedisOfficialPrimaryFailureStore(cfg RedisOfficialPrimaryFailureStoreConfig) (OfficialPrimaryFailureStore, error) {
	rawURL := strings.TrimSpace(cfg.URL)
	if rawURL == "" {
		return nil, errors.New("redis url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(parsed.Scheme)) != "redis" {
		return nil, fmt.Errorf("unsupported redis url scheme %q", parsed.Scheme)
	}
	hostname := strings.TrimSpace(parsed.Hostname())
	if hostname == "" {
		return nil, errors.New("redis host is required")
	}
	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		port = "6379"
	}
	host := net.JoinHostPort(hostname, port)
	db := 0
	pathDB := strings.TrimSpace(strings.TrimPrefix(parsed.Path, "/"))
	if pathDB != "" {
		parsedDB, parseErr := strconv.Atoi(pathDB)
		if parseErr != nil {
			return nil, fmt.Errorf("parse redis db from path: %w", parseErr)
		}
		if parsedDB < 0 {
			return nil, errors.New("redis db must be non-negative")
		}
		db = parsedDB
	}
	if queryDB := strings.TrimSpace(parsed.Query().Get("db")); queryDB != "" {
		parsedDB, parseErr := strconv.Atoi(queryDB)
		if parseErr != nil {
			return nil, fmt.Errorf("parse redis db from query: %w", parseErr)
		}
		if parsedDB < 0 {
			return nil, errors.New("redis db must be non-negative")
		}
		db = parsedDB
	}
	username := ""
	password := ""
	if parsed.User != nil {
		username = strings.TrimSpace(parsed.User.Username())
		password, _ = parsed.User.Password()
		password = strings.TrimSpace(password)
	}
	dialTimeout := cfg.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 2 * time.Second
	}
	ioTimeout := cfg.IOTimeout
	if ioTimeout <= 0 {
		ioTimeout = 2 * time.Second
	}
	return &redisOfficialPrimaryFailureStore{
		addr:        host,
		username:    username,
		password:    password,
		db:          db,
		keyPrefix:   normalizeRedisPrimaryFailureKeyPrefix(cfg.KeyPrefix),
		dialTimeout: dialTimeout,
		ioTimeout:   ioTimeout,
	}, nil
}

func (s *redisOfficialPrimaryFailureStore) ShouldSkipPrimaryDuringCooldown(ctx context.Context, modelID string, now time.Time) (bool, error) {
	key := normalizePrimaryFailureStateKey(modelID)
	if key == "" {
		return false, nil
	}
	cooldownUntil, hasCooldown, err := s.getInt64(ctx, s.cooldownKey(key))
	if err != nil {
		return false, err
	}
	if !hasCooldown {
		return false, nil
	}
	if now.UnixNano() < cooldownUntil {
		return true, nil
	}
	_, _ = s.do(ctx, "DEL", s.cooldownKey(key))
	return false, nil
}

func (s *redisOfficialPrimaryFailureStore) RecordPrimaryFailure(
	ctx context.Context,
	modelID string,
	now time.Time,
	failureThreshold int,
	cooldown time.Duration,
) (OfficialPrimaryFailureRecordResult, error) {
	key := normalizePrimaryFailureStateKey(modelID)
	if key == "" {
		return OfficialPrimaryFailureRecordResult{}, nil
	}
	if failureThreshold <= 0 {
		failureThreshold = officialPrimaryFailureThreshold
	}
	if cooldown <= 0 {
		cooldown = officialPrimaryCooldownDuration
	}
	failures, err := s.incr(ctx, s.failuresKey(key))
	if err != nil {
		return OfficialPrimaryFailureRecordResult{}, err
	}
	result := OfficialPrimaryFailureRecordResult{
		ConsecutiveFailures: int(failures),
	}
	if int(failures) < failureThreshold {
		return result, nil
	}

	nowUnixNano := now.UnixNano()
	existingCooldownUntil, hasCooldown, err := s.getInt64(ctx, s.cooldownKey(key))
	if err != nil {
		return OfficialPrimaryFailureRecordResult{}, err
	}
	result.CooldownOpened = !hasCooldown || existingCooldownUntil <= nowUnixNano

	cooldownUntil := now.Add(cooldown)
	if _, err := s.do(ctx, "SET", s.cooldownKey(key), strconv.FormatInt(cooldownUntil.UnixNano(), 10), "PX", strconv.FormatInt(cooldown.Milliseconds(), 10)); err != nil {
		return OfficialPrimaryFailureRecordResult{}, err
	}
	if _, err := s.do(ctx, "PEXPIRE", s.failuresKey(key), strconv.FormatInt(cooldown.Milliseconds(), 10)); err != nil {
		return OfficialPrimaryFailureRecordResult{}, err
	}
	result.CooldownUntil = cooldownUntil
	return result, nil
}

func (s *redisOfficialPrimaryFailureStore) ClearPrimaryFailures(ctx context.Context, modelID string) (bool, error) {
	key := normalizePrimaryFailureStateKey(modelID)
	if key == "" {
		return false, nil
	}
	existsCount, err := s.exists(ctx, s.failuresKey(key), s.cooldownKey(key))
	if err != nil {
		return false, err
	}
	if existsCount > 0 {
		if _, err := s.do(ctx, "DEL", s.failuresKey(key), s.cooldownKey(key)); err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

func (s *redisOfficialPrimaryFailureStore) PruneModelStates(ctx context.Context, activeModelIDs []string) error {
	_ = ctx
	_ = activeModelIDs
	// This store keeps only small expiring keys. Pruning unknown model keys is optional.
	return nil
}

func (s *redisOfficialPrimaryFailureStore) failuresKey(modelID string) string {
	return s.keyPrefix + "failures:" + modelID
}

func (s *redisOfficialPrimaryFailureStore) cooldownKey(modelID string) string {
	return s.keyPrefix + "cooldown:" + modelID
}

func normalizeRedisPrimaryFailureKeyPrefix(raw string) string {
	prefix := strings.TrimSpace(raw)
	if prefix == "" {
		return defaultRedisPrimaryFailureKeyPrefix
	}
	if strings.HasSuffix(prefix, ":") {
		return prefix
	}
	return prefix + ":"
}

func (s *redisOfficialPrimaryFailureStore) incr(ctx context.Context, key string) (int64, error) {
	resp, err := s.do(ctx, "INCR", key)
	if err != nil {
		return 0, err
	}
	if resp.kind != ':' {
		return 0, fmt.Errorf("redis INCR unexpected response: %q", resp.kind)
	}
	return resp.integer, nil
}

func (s *redisOfficialPrimaryFailureStore) exists(ctx context.Context, keys ...string) (int64, error) {
	args := make([]string, 1, 1+len(keys))
	args[0] = "EXISTS"
	args = append(args, keys...)
	resp, err := s.do(ctx, args...)
	if err != nil {
		return 0, err
	}
	if resp.kind != ':' {
		return 0, fmt.Errorf("redis EXISTS unexpected response: %q", resp.kind)
	}
	return resp.integer, nil
}

func (s *redisOfficialPrimaryFailureStore) getInt64(ctx context.Context, key string) (int64, bool, error) {
	resp, err := s.do(ctx, "GET", key)
	if err != nil {
		return 0, false, err
	}
	if resp.isNil {
		return 0, false, nil
	}
	value, parseErr := strconv.ParseInt(strings.TrimSpace(resp.stringValue), 10, 64)
	if parseErr != nil {
		_, _ = s.do(ctx, "DEL", key)
		return 0, false, nil
	}
	return value, true, nil
}

type redisResponse struct {
	kind        byte
	stringValue string
	integer     int64
	isNil       bool
}

func (s *redisOfficialPrimaryFailureStore) do(ctx context.Context, args ...string) (redisResponse, error) {
	if len(args) == 0 {
		return redisResponse{}, errors.New("redis command is required")
	}
	dialer := &net.Dialer{Timeout: s.dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return redisResponse{}, err
	}
	defer conn.Close()
	deadline := time.Now().Add(s.ioTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return redisResponse{}, err
	}

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	if err := s.authAndSelect(reader, writer); err != nil {
		return redisResponse{}, err
	}
	if err := writeRedisCommand(writer, args...); err != nil {
		return redisResponse{}, err
	}
	if err := writer.Flush(); err != nil {
		return redisResponse{}, err
	}
	resp, err := readRedisResponse(reader)
	if err != nil {
		return redisResponse{}, err
	}
	if resp.kind == '-' {
		return redisResponse{}, errors.New(resp.stringValue)
	}
	return resp, nil
}

func (s *redisOfficialPrimaryFailureStore) authAndSelect(
	reader *bufio.Reader,
	writer *bufio.Writer,
) error {
	if s.password != "" {
		authArgs := []string{"AUTH"}
		if s.username != "" {
			authArgs = append(authArgs, s.username, s.password)
		} else {
			authArgs = append(authArgs, s.password)
		}
		if err := writeRedisCommand(writer, authArgs...); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		resp, err := readRedisResponse(reader)
		if err != nil {
			return err
		}
		if resp.kind == '-' {
			return errors.New(resp.stringValue)
		}
	}
	if s.db > 0 {
		if err := writeRedisCommand(writer, "SELECT", strconv.Itoa(s.db)); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		resp, err := readRedisResponse(reader)
		if err != nil {
			return err
		}
		if resp.kind == '-' {
			return errors.New(resp.stringValue)
		}
	}
	return nil
}

func writeRedisCommand(writer *bufio.Writer, args ...string) error {
	if _, err := writer.WriteString(fmt.Sprintf("*%d\r\n", len(args))); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := writer.WriteString(fmt.Sprintf("$%d\r\n", len(arg))); err != nil {
			return err
		}
		if _, err := writer.WriteString(arg); err != nil {
			return err
		}
		if _, err := writer.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return nil
}

func readRedisResponse(reader *bufio.Reader) (redisResponse, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return redisResponse{}, err
	}
	switch prefix {
	case '+':
		line, err := readRedisLine(reader)
		if err != nil {
			return redisResponse{}, err
		}
		return redisResponse{kind: '+', stringValue: line}, nil
	case '-':
		line, err := readRedisLine(reader)
		if err != nil {
			return redisResponse{}, err
		}
		return redisResponse{kind: '-', stringValue: line}, nil
	case ':':
		line, err := readRedisLine(reader)
		if err != nil {
			return redisResponse{}, err
		}
		value, parseErr := strconv.ParseInt(line, 10, 64)
		if parseErr != nil {
			return redisResponse{}, parseErr
		}
		return redisResponse{kind: ':', integer: value}, nil
	case '$':
		line, err := readRedisLine(reader)
		if err != nil {
			return redisResponse{}, err
		}
		length, parseErr := strconv.Atoi(line)
		if parseErr != nil {
			return redisResponse{}, parseErr
		}
		if length < 0 {
			return redisResponse{kind: '$', isNil: true}, nil
		}
		buf := make([]byte, length+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return redisResponse{}, err
		}
		return redisResponse{kind: '$', stringValue: string(buf[:length])}, nil
	default:
		return redisResponse{}, fmt.Errorf("unsupported redis response prefix: %q", prefix)
	}
}

func readRedisLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, nil
}
