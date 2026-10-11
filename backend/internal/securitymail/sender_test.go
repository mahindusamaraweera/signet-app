package securitymail

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"signet-backend/internal/config"
)

func TestRejectsUnsafeHeadersWithoutConnecting(t *testing.T) {
	for _, destination := range []string{"bad", "user@example.com\r\nBcc: other@example.com", "Name <user@example.com>"} {
		if err := New(&config.Config{MailFrom: "sender@example.com"}).Send(context.Background(), destination, "Security", "secret"); err == nil {
			t.Fatal("invalid destination accepted")
		}
	}
	if err := New(&config.Config{MailFrom: "sender@example.com"}).Send(context.Background(), "user@example.com", "Security\r\nBcc: bad", "secret"); err == nil {
		t.Fatal("header injection accepted")
	}
}

func TestProductionRejectsUnencryptedSMTPBeforeSendingCode(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	commands := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			commands <- "accept failed"
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		conn.Write([]byte("220 local SMTP\r\n"))
		reader := bufio.NewReader(conn)
		greeting, _ := reader.ReadString('\n')
		conn.Write([]byte("250 local SMTP\r\n"))
		next, _ := reader.ReadString('\n')
		commands <- greeting + next
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	err = New(&config.Config{MailHost: host, MailPort: port, MailFrom: "sender@example.com", AppEnv: "production"}).Send(context.Background(), "user@example.com", "Security", "secret-code")
	if err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("expected TLS refusal, got %v", err)
	}
	if got := <-commands; strings.Contains(got, "MAIL") || strings.Contains(got, "secret-code") {
		t.Fatal("sent challenge on plaintext transport")
	}
}

func TestCanceledDeliveryFailsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := New(&config.Config{MailHost: "127.0.0.1", MailPort: "1", MailFrom: "sender@example.com"}).Send(ctx, "user@example.com", "Security", "secret-code")
	if err == nil || strings.Contains(err.Error(), "secret-code") {
		t.Fatal("unsafe cancellation result")
	}
}
