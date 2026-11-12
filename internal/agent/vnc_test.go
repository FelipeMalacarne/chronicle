package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// fakeVNC is an x11vnc stand-in on loopback: it greets like RFB and then
// echoes what it reads.
func fakeVNC(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = c.Write([]byte("RFB 003.008\n"))
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}

func TestVNCThroughTheLink(t *testing.T) {
	link := linkedWith(t, &fakeGame{}, fakeVNC(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := link.OpenVNC(ctx, "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = stream.Close() }()
	_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
	greet := make([]byte, 12)
	if _, err = io.ReadFull(stream, greet); err != nil || string(greet) != "RFB 003.008\n" {
		t.Fatalf("greeting = %q %v", greet, err)
	}
	// Bytes go both ways, in order, across message boundaries.
	payload := bytes.Repeat([]byte("frame-"), 20000)
	go func() { _, _ = stream.Write(payload) }()
	got := make([]byte, len(payload))
	if _, err = io.ReadFull(stream, got); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("echo: %d bytes, err %v", len(got), err)
	}
}

func TestVNCWithoutX11vnc(t *testing.T) {
	// Nothing listens: the agent refuses and the server gets its error.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	link := linkedWith(t, &fakeGame{}, addr)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var cmd *CommandError
	if _, err := link.OpenVNC(ctx, ""); !errors.As(err, &cmd) {
		t.Fatalf("open without x11vnc = %v", err)
	}
}
