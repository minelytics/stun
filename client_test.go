package stun

import (
	"net"
	"testing"
	"time"
)

type stubMultiplexer struct {
	buf      []byte
	response []byte
	addr     net.Addr
	clients  []multiplexClient
}

func (m *stubMultiplexer) Add(f MultiplexFunc, c func([]byte, net.Addr)) {
	m.clients = append(m.clients, multiplexClient{
		f: f,
		c: c,
	})
}

func (m *stubMultiplexer) WriteTo(b []byte, addr net.Addr) (int, error) {
	m.buf = append(m.buf[:0], b...)
	m.addr = addr
	for _, client := range m.clients {
		if !client.f(m.response) {
			continue
		}
		client.c(m.response, m.addr)
	}
	return len(b), nil
}

func BenchmarkClient_AddTransaction(b *testing.B) {
	b.ReportAllocs()
	c := &Client{}
	id := transactionID{}
	for i := 0; i < b.N; i++ {
		c.addTransaction(id)
	}
}

func BenchmarkClient_Do(b *testing.B) {
	b.ReportAllocs()
	client := &Client{}
	m := &stubMultiplexer{
		addr: &net.UDPAddr{
			IP: net.IPv4(1, 2, 3, 4),
		},
	}
	client.Multiplex(m)
	if err := client.Dial(m.addr); err != nil {
		b.Error(err)
	}

	var (
		response = MustBuild(TransactionID, BindingSuccess)
		request  = MustBuild(NewTransactionIDSetter(response.TransactionID), BindingRequest)
	)
	m.response = response.Raw

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.Do(request, func(res *Message) error {
			return nil
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestClient_Do(t *testing.T) {
	client := &Client{}
	m := &stubMultiplexer{
		addr: &net.UDPAddr{
			IP: net.IPv4(1, 2, 3, 4),
		},
	}
	client.Multiplex(m)
	if err := client.Dial(m.addr); err != nil {
		t.Error(err)
	}
	request, err := Build(TransactionID, BindingRequest)
	if err != nil {
		t.Fatal(err)
	}
	response, err := Build(NewTransactionIDSetter(request.TransactionID), BindingSuccess)
	if err != nil {
		t.Fatal(err)
	}
	m.response = response.Raw
	if err := client.Do(request, func(res *Message) error {
		if !res.Equal(response) {
			t.Error("not equal")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAgent_Process(t *testing.T) {
	a := NewAgent(AgentOptions{})
	if err := a.Close(); err != nil {
		t.Error(err)
	}
}

func TestAgent_Start(t *testing.T) {
	a := NewAgent(AgentOptions{})
	id := NewTransactionID()
	deadline := time.Now().AddDate(0, 0, 1)
	if err := a.Start(id, deadline, noopHandler); err != nil {
		t.Errorf("failed to statt transaction: %s", err)
	}
	if err := a.Start(id, deadline, noopHandler); err != ErrTransactionExists {
		t.Errorf("duplicate start should return <%s>, got <%s>",
			ErrTransactionExists, err,
		)
	}
	if err := a.Close(); err != nil {
		t.Error(err)
	}
	id = NewTransactionID()
	if err := a.Start(id, deadline, noopHandler); err != ErrAgentClosed {
		t.Errorf("start on closed agent should return <%s>, got <%s>",
			ErrAgentClosed, err,
		)
	}
}

func TestAgent_Stop(t *testing.T) {
	a := NewAgent(AgentOptions{})
	if err := a.Stop(transactionID{}); err != ErrTransactionNotExists {
		t.Fatalf("unexpected error: %s, should be %s", err, ErrTransactionNotExists)
	}
	id := NewTransactionID()
	called := make(chan AgentEvent, 1)
	timeout := time.Millisecond * 200
	if err := a.Start(id, time.Now().Add(timeout), func(e AgentEvent) {
		called <- e
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(id); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-called:
		if e.Error != ErrTransactionStopped {
			t.Fatalf("unexpected error: %s, should be %s",
				e.Error, ErrTransactionStopped,
			)
		}
	case <-time.After(timeout * 2):
		t.Fatal("timed out")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(transactionID{}); err != ErrAgentClosed {
		t.Fatalf("unexpected error: %s, should be %s", err, ErrAgentClosed)
	}
}

var noopHandler = func(e AgentEvent) {}

func TestAgent_GC(t *testing.T) {
	a := NewAgent(AgentOptions{
		Handler: noopHandler,
	})
	shouldTimeOut := func(e AgentEvent) {
		if e.Error != ErrTransactionTimeOut {
			t.Errorf("should time out, but got <%s>", e.Error)
		}
	}
	shouldNotTimeOut := func(e AgentEvent) {
		if e.Error == ErrTransactionTimeOut {
			t.Error("should not time out")
		}
	}
	deadline := time.Date(2027, time.November, 21,
		23, 13, 34, 120021,
		time.UTC,
	)
	gcDeadline := deadline.Add(time.Second)
	deadlineNotGC := gcDeadline.AddDate(0, 0, 1)
	for i := 0; i < 5; i++ {
		if err := a.Start(NewTransactionID(), deadline, shouldTimeOut); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		if err := a.Start(NewTransactionID(), deadlineNotGC, shouldNotTimeOut); err != nil {
			t.Fatal(err)
		}
	}
	a.garbageCollect(gcDeadline)
	if err := a.Close(); err != nil {
		t.Error(err)
	}
	// Should not panic:
	a.garbageCollect(gcDeadline)
}

func BenchmarkAgent_GC(b *testing.B) {
	a := NewAgent(AgentOptions{
		Handler: noopHandler,
	})
	deadline := time.Now().AddDate(0, 0, 1)
	for i := 0; i < agentGCInitCap; i++ {
		if err := a.Start(NewTransactionID(), deadline, noopHandler); err != nil {
			b.Fatal(err)
		}
	}
	defer func() {
		if err := a.Close(); err != nil {
			b.Error(err)
		}
	}()
	b.ReportAllocs()
	gcDeadline := deadline.Add(-time.Second)
	for i := 0; i < b.N; i++ {
		a.garbageCollect(gcDeadline)
	}
}

func BenchmarkAgent_Process(b *testing.B) {
	a := NewAgent(AgentOptions{
		Handler: noopHandler,
	})
	deadline := time.Now().AddDate(0, 0, 1)
	for i := 0; i < 1000; i++ {
		if err := a.Start(NewTransactionID(), deadline, noopHandler); err != nil {
			b.Fatal(err)
		}
	}
	defer func() {
		if err := a.Close(); err != nil {
			b.Error(err)
		}
	}()
	b.ReportAllocs()
	ev := AgentProcessArgs{
		Message: MustBuild(
			TransactionID,
		),
	}
	for i := 0; i < b.N; i++ {
		if err := a.Process(ev); err != nil {
			b.Fatal(err)
		}
	}
}
