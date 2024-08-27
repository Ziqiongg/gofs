package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"module/placeholder/internal/config"
	"module/placeholder/internal/db"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Server struct {
	r       *http.ServeMux
	srv     *http.Server
	conf    config.Config
	db      *db.DB
	closeFn []func(context.Context) error
}

func New(conf config.Config) (*Server, error) {
	s := new(Server)
	s.conf = conf
	s.r = http.NewServeMux()
	s.srv = &http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		Addr:         fmt.Sprintf("%s:%s", conf.Host, conf.Port),
		Handler:      s.r,
	}
	var err error
	s.db, err = s.initDb()
	if err != nil {
		return nil, err
	}
	err = db.MigrateTables(s.db)
	if err != nil {
		return nil, fmt.Errorf("db: migrating tables: %w", err)
	}

	err = s.initTelemetry()
	if err != nil {
		return nil, fmt.Errorf("server: initializing telemetry: %w", err)
	}
	return s, nil
}

func (s *Server) initDb() (*db.DB, error) {
	switch {
	case s.conf.Env.Local() && s.conf.DSN != "":
		return db.LocalPG(s.conf.DSN)
	case s.conf.Env.Dev() && s.conf.DSN != "":
		return db.CloudSQL(s.conf.DSN, s.conf.ICN)
	case s.conf.Env.Prod() && s.conf.DSN != "":
		return db.CloudSQL(s.conf.DSN, s.conf.ICN)
	default:
		log.Println("server: no database connection")
		return nil, nil
	}
}

func (s *Server) initTelemetry() error {
	exporter, err := zipkin.New(
		"http://localhost:9411/api/v2/spans",
	)
	if err != nil {
		return err
	}

	batcher := trace.NewBatchSpanProcessor(exporter)

	tp := trace.NewTracerProvider(
		trace.WithSpanProcessor(batcher),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(s.conf.ServiceName),
		)),
	)
	otel.SetTracerProvider(tp)

	s.closeFn = append(s.closeFn, tp.Shutdown)
	return nil
}

func (s *Server) ListenAndServe() error {
	s.Routes()
	// address for use when testing cookies locally
	if s.conf.Host == "0.0.0.0" {
		log.Printf("Server: listening on http://localhost:%s", s.conf.Port)
	} else {
		log.Printf("Server: listening on http://%s", s.srv.Addr)
	}
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	defer s.srv.Shutdown(ctx)
	for _, fn := range s.closeFn {
		if err := fn(ctx); err != nil {
			log.Println("server: error closing resources:", err)
		}
	}
	return nil
}
