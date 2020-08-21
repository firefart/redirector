package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const (
	defaultGracefulTimeout = 5 * time.Second
)

var (
	debugOutput bool
	redirect    string
)

type application struct{}

func main() {
	var host string
	var wait time.Duration
	flag.StringVar(&host, "host", "0.0.0.0:8080", "IP and Port to bind to")
	flag.StringVar(&redirect, "redirect", "https://google.com", "redirect target")
	flag.BoolVar(&debugOutput, "debug", false, "Enable DEBUG mode")
	flag.DurationVar(&wait, "graceful-timeout", defaultGracefulTimeout, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	log.SetOutput(os.Stdout)
	if debugOutput {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	app := &application{}

	srv := &http.Server{
		Addr:    host,
		Handler: app.routes(),
	}
	log.Infof("Starting server on %s", host)
	if debugOutput {
		log.Debug("DEBUG mode enabled")
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error(err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT)
	signal.Notify(c, syscall.SIGTERM)
	<-c
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	log.Info("shutting down")
	os.Exit(0)
}

func (app *application) routes() http.Handler {
	r := mux.NewRouter()
	r.Use(app.loggingMiddleware)
	r.Use(app.recoverPanic)
	r.PathPrefix("/").HandlerFunc(app.catchAllHandler)
	return r
}

func (app *application) catchAllHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, redirect, http.StatusMovedPermanently)
}

func (app *application) loggingMiddleware(next http.Handler) http.Handler {
	return handlers.CombinedLoggingHandler(os.Stdout, next)
}

func (app *application) logError(w http.ResponseWriter, err error, withTrace bool) {
	w.Header().Set("Connection", "close")
	errorText := fmt.Sprintf("%v", err)
	log.Error(errorText)
	if withTrace {
		log.Errorf("%s", debug.Stack())
	}
	http.Error(w, "There was an error processing your request", http.StatusInternalServerError)
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				app.logError(w, fmt.Errorf("%s", err), true)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
