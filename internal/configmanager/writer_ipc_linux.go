//go:build linux

package configmanager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

func validDigest(s string) bool {
	decoded, err := hex.DecodeString(s)
	return err == nil && len(decoded) == 32 && s == strings.ToLower(s)
}

type writerRequest struct {
	Version  int    `json:"version"`
	Action   string `json:"action"`
	Resource string `json:"resource,omitempty"`
	Patch    *Patch `json:"patch,omitempty"`
	Token    string `json:"token,omitempty"`
	Expected string `json:"expected,omitempty"`
}
type writerReply struct {
	Data  []byte `json:"data,omitempty"`
	Token string `json:"token,omitempty"`
	Error string `json:"error,omitempty"`
}

func (s *FileWriterServer) dispatch(uid uint32, q writerRequest) writerReply {
	if q.Version != 1 {
		return writerReply{Error: "invalid writer version"}
	}
	var out writerReply
	var err error
	switch q.Action {
	case "read", "latest":
		if q.Patch != nil || q.Token != "" || q.Expected != "" {
			return writerReply{Error: "invalid writer shape"}
		}
		if _, err = s.resource(uid, q.Resource); err != nil {
			break
		}
		if q.Action == "read" {
			out.Data, err = s.read(uid, q.Resource)
		} else {
			var names []string
			names, err = s.snapshots(q.Resource)
			if err == nil {
				if len(names) == 0 {
					err = fmt.Errorf("configwriter: no backup")
				} else {
					out.Token = names[len(names)-1]
				}
			}
		}
	case "commit":
		if q.Resource != "" || q.Patch == nil || q.Token != "" || !validDigest(q.Expected) || !validDigest(q.Patch.BaseVersion) {
			return writerReply{Error: "invalid writer shape"}
		}
		out.Token, err = s.commit(uid, *q.Patch, q.Expected)
	case "restore":
		if q.Patch != nil || q.Token == "" || !validDigest(q.Expected) {
			return writerReply{Error: "invalid writer shape"}
		}
		err = s.restore(uid, q.Resource, q.Token, q.Expected)
	default:
		err = fmt.Errorf("configwriter: invalid action")
	}
	if err != nil {
		out = writerReply{Error: "configwriter: request rejected"}
	} // Never return raw values or paths from root errors.
	return out
}

func (s *FileWriterServer) Serve(ctx context.Context, path string) error {
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err
	}
	defer l.Close()
	if err := os.Chmod(path, 0660); err != nil {
		return err
	}
	go func() { <-ctx.Done(); _ = l.Close() }()
	slots := make(chan struct{}, 4)
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = conn.Close()
			continue
		}
		go func() {
			defer func() { <-slots }()
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
			uid, err := peerUID(conn)
			if err != nil {
				return
			}
			if _, ok := s.peers[uid]; !ok {
				return
			}
			frame, err := bufio.NewReader(io.LimitReader(conn, maxIPCMessageBytes+1)).ReadBytes('\n')
			if err != nil || len(frame) > maxIPCMessageBytes {
				return
			}
			if _, err := parseJSON(frame); err != nil {
				return
			}
			var q writerRequest
			decoder := json.NewDecoder(bytes.NewReader(frame))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&q) != nil {
				return
			}
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				return
			}
			out := s.dispatch(uid, q)
			_ = json.NewEncoder(conn).Encode(out)
		}()
	}
}

type FileWriterClient struct {
	Socket string
	UID    uint32
}

func (c *FileWriterClient) call(q writerRequest) (writerReply, error) {
	q.Version = 1
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return writerReply{}, err
	}
	defer conn.Close()
	un, ok := conn.(*net.UnixConn)
	if !ok {
		return writerReply{}, fmt.Errorf("configwriter: invalid connection")
	}
	uid, err := peerUID(un)
	if err != nil || uid != c.UID {
		return writerReply{}, fmt.Errorf("configwriter: untrusted writer")
	}
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	if err := json.NewEncoder(conn).Encode(q); err != nil {
		return writerReply{}, err
	}
	frame, err := bufio.NewReader(io.LimitReader(conn, 6<<20+1)).ReadBytes('\n')
	if err != nil || len(frame) > 6<<20 {
		return writerReply{}, fmt.Errorf("configwriter: invalid reply")
	}
	var reply writerReply
	if err := json.Unmarshal(frame, &reply); err != nil {
		return reply, err
	}
	if reply.Error != "" {
		return reply, fmt.Errorf("%s", reply.Error)
	}
	return reply, nil
}
func (c *FileWriterClient) Read(id string) ([]byte, error) {
	r, e := c.call(writerRequest{Action: "read", Resource: id})
	return r.Data, e
}
func (c *FileWriterClient) Commit(p Patch, want string) (string, error) {
	r, e := c.call(writerRequest{Action: "commit", Patch: &p, Expected: want})
	return r.Token, e
}
func (c *FileWriterClient) Restore(id, token, expected string) error {
	_, e := c.call(writerRequest{Action: "restore", Resource: id, Token: token, Expected: expected})
	return e
}
func (c *FileWriterClient) Latest(id string) (string, error) {
	r, e := c.call(writerRequest{Action: "latest", Resource: id})
	return r.Token, e
}
