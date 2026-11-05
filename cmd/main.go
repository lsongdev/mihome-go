// Command cmd serves the HTTP API and the web panel.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lsongdev/mihome-go/api"
	"github.com/lsongdev/mihome-go/mihome"
	"github.com/lsongdev/mihome-go/store"
	"github.com/lsongdev/mihome-go/web"
)

// loadEnv sets the variables of a .env file, if there is one. Variables already
// set in the environment take precedence.
func loadEnv(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if _, exists := os.LookupEnv(key); !exists {
			value = strings.Trim(strings.TrimSpace(value), `"'`)
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func run() error {
	dir := flag.String("d", ".", "data directory: holds config.yaml, the database and the login state")
	flag.Parse()
	if err := loadEnv(filepath.Join(*dir, ".env")); err != nil {
		return err
	}
	config, err := loadConfig(*dir)
	if err != nil {
		return fmt.Errorf("load configuration from %s: %w", *dir, err)
	}
	token := config.apiToken()
	if token == "" && !web.IsLoopback(config.API.Listen) {
		return fmt.Errorf("listen address %s is not loopback: set an API token", config.API.Listen)
	}
	db, err := store.Open(config.Database.Path)
	if err != nil {
		return err
	}
	defer db.Close()
	client := mihome.NewClient(config.username(), config.password())
	client.TokenPath = config.Account.TokenPath

	handler := api.New(client, api.Options{
		Cache:      db,
		DevicesTTL: config.Cache.DevicesTTL,
		SpecTTL:    config.Cache.SpecTTL,
	})
	server := &http.Server{
		Addr:              config.API.Listen,
		Handler:           web.New(handler).Standalone(token),
		ReadHeaderTimeout: 5 * time.Second,
	}
	failed := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			failed <- err
		}
	}()
	log.Printf("Web UI: http://%s, data directory: %s", config.API.Listen, *dir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
