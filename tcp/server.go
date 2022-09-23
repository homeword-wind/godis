package tcp

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"godis/interface/tcp"
	"godis/lib/logger"
)

// Conf stores tcp server properties
type Conf struct {
	Addr    string        `yaml:"address"`
	MaxConn uint32        `yaml:"max-connect"`
	Timeout time.Duration `yaml:"timeout"`
}

// ListenAndServe binds port and handle requests, blocking until close
func ListenAndServe(listener net.Listener, handler tcp.Handler, closeCh <-chan struct{}) {
	// listen signal
	go func() {
		<-closeCh
		logger.Info("shutting down...")
		// stop monitoring
		// listener.Accept() will return io.EOF immediately
		_ = listener.Close()
		_ = handler.Close()
	}()

	// listen port
	defer func() {
		// close during unexpected error
		_ = listener.Close()
		_ = handler.Close()
	}()

	ctx := context.Background()
	var wg sync.WaitGroup
	for {
		// monitor port
		// blocking util a new connection is received or an error occurs
		conn, err := listener.Accept()
		if err != nil {
			break
		}

		logger.Info("accept link")
		wg.Add(1)

		go func() {
			defer func() {
				wg.Done()
			}()

			handler.Handle(ctx, conn)
		}()
	}
}

// ListenAndServeWithSignal binds port and handle requests, blocking until receive stop signal
func ListenAndServeWithSignal(conf *Conf, handler tcp.Handler) error {
	closeCh := make(chan struct{})
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			closeCh <- struct{}{}
		}
	}()

	listener, err := net.Listen("tcp", conf.Addr)
	if err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("bind: %s, start listening...", conf.Addr))
	ListenAndServe(listener, handler, closeCh)
	return nil
}
