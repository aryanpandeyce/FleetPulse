// Package redis implements a lightweight, zero-dependency Redis client
// communicating via standard Redis Serialization Protocol (RESP).
package redis

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrNilReply     = errors.New("redis: nil returned")
	ErrClosed       = errors.New("redis: client is closed")
	ErrScriptFailed = errors.New("redis: script evaluation error")
)

// Client represents a thread-safe connection to a Redis server.
type Client struct {
	addr    string
	timeout time.Duration
	mu      sync.Mutex
	conn    net.Conn
	reader  *bufio.Reader
	closed  bool
}

// NewClient creates a new Redis client connecting to addr (e.g. "localhost:6379").
func NewClient(addr string) *Client {
	if addr == "" {
		addr = "localhost:6379"
	}
	// Strip "redis://" prefix if present
	addr = strings.TrimPrefix(addr, "redis://")
	return &Client{
		addr:    addr,
		timeout: 3 * time.Second,
	}
}

// Ping checks if the Redis server is responsive.
func (c *Client) Ping(ctx context.Context) error {
	res, err := c.Do(ctx, "PING")
	if err != nil {
		return err
	}
	if str, ok := res.(string); ok && str == "PONG" {
		return nil
	}
	return nil
}

// ScriptLoad loads a Lua script into Redis and returns its SHA1 digest.
func (c *Client) ScriptLoad(ctx context.Context, script string) (string, error) {
	res, err := c.Do(ctx, "SCRIPT", "LOAD", script)
	if err != nil {
		return "", err
	}
	if sha, ok := res.(string); ok {
		return sha, nil
	}
	return fmt.Sprintf("%v", res), nil
}

// EvalSha executes a cached Lua script by its SHA1 hash.
func (c *Client) EvalSha(ctx context.Context, sha string, keys []string, args ...interface{}) (interface{}, error) {
	cmdArgs := make([]interface{}, 0, 3+len(keys)+len(args))
	cmdArgs = append(cmdArgs, "EVALSHA", sha, len(keys))
	for _, k := range keys {
		cmdArgs = append(cmdArgs, k)
	}
	cmdArgs = append(cmdArgs, args...)
	return c.Do(ctx, cmdArgs...)
}

// Do sends a generic command to Redis and parses the RESP response.
func (c *Client) Do(ctx context.Context, args ...interface{}) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClosed
	}

	if err := c.ensureConn(); err != nil {
		return nil, err
	}

	// Set deadline
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.timeout)
	}
	_ = c.conn.SetDeadline(deadline)

	// Encode command in RESP array format: *<count>\r\n$<len>\r\n<arg>\r\n...
	var buf strings.Builder
	buf.WriteString("*" + strconv.Itoa(len(args)) + "\r\n")
	for _, arg := range args {
		str := fmt.Sprintf("%v", arg)
		buf.WriteString("$" + strconv.Itoa(len(str)) + "\r\n" + str + "\r\n")
	}

	if _, err := io.WriteString(c.conn, buf.String()); err != nil {
		c.closeConn()
		return nil, fmt.Errorf("redis write failed: %w", err)
	}

	// Read reply
	reply, err := c.readReply()
	if err != nil {
		c.closeConn()
		return nil, fmt.Errorf("redis read failed: %w", err)
	}

	return reply, nil
}

// Close closes the underlying network connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.closeConn()
}

func (c *Client) ensureConn() error {
	if c.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to redis at %s: %w", c.addr, err)
	}
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	return nil
}

func (c *Client) closeConn() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.reader = nil
		return err
	}
	return nil
}

// readReply parses a single RESP protocol message.
func (c *Client) readReply() (interface{}, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(line, "\r\n")
	if len(line) == 0 {
		return nil, errors.New("empty RESP line")
	}

	switch line[0] {
	case '+': // Simple String: +OK
		return line[1:], nil
	case '-': // Error: -ERR
		return nil, errors.New(line[1:])
	case ':': // Integer: :100
		return strconv.ParseInt(line[1:], 10, 64)
	case '$': // Bulk String: $<len>\r\n<data>\r\n
		length, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if length == -1 {
			return nil, ErrNilReply
		}
		data := make([]byte, length+2) // data + \r\n
		if _, err := io.ReadFull(c.reader, data); err != nil {
			return nil, err
		}
		return string(data[:length]), nil
	case '*': // Array: *<count>\r\n...
		count, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if count == -1 {
			return nil, ErrNilReply
		}
		items := make([]interface{}, count)
		for i := 0; i < count; i++ {
			item, err := c.readReply()
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unknown RESP prefix: %c", line[0])
	}
}
