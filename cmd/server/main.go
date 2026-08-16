// Befehl server startet den Sitzungsserver: Saal einlesen, Zustand halten,
// Clients bedienen, Kamera fahren.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tim-rose-hnvr/kameranachverfolgung/intern/kamera"
	"github.com/tim-rose-hnvr/kameranachverfolgung/intern/kern"
	"github.com/tim-rose-hnvr/kameranachverfolgung/intern/speicher"
	"github.com/tim-rose-hnvr/kameranachverfolgung/intern/web"
)

const (
	migrationsDatei = "migrationen/001_grundlage.sql"
	webVerzeichnis  = "web"
)

func main() {
	konfigPfad := flag.String("konfiguration", "config.yaml", "Pfad zu config.yaml")
	flag.Parse()

	protokoll := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(protokoll)

	if err := starten(*konfigPfad, protokoll); err != nil {
		protokoll.Error("server beendet", "grund", err)
		os.Exit(1)
	}
}

func starten(konfigPfad string, protokoll *slog.Logger) error {
	konfiguration, err := KonfigurationLesen(konfigPfad)
	if err != nil {
		return err
	}

	ctx, halt := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer halt()

	saaldaten, err := speicher.SaalLesen(konfiguration.SaalDatei)
	if err != nil {
		return err
	}

	anlaufCtx, anlaufFertig := context.WithTimeout(ctx, 30*time.Second)
	defer anlaufFertig()

	ablage, err := speicher.Verbinden(anlaufCtx, konfiguration.Datenbank)
	if err != nil {
		return err
	}
	defer ablage.Schliessen()

	migration, err := os.ReadFile(migrationsDatei)
	if err != nil {
		return err
	}
	if err := ablage.SchemaSicherstellen(anlaufCtx, string(migration)); err != nil {
		return err
	}

	saalID, aufbau, err := ablage.SaalImportieren(anlaufCtx, saaldaten)
	if err != nil {
		return err
	}
	protokoll.Info("saal eingelesen",
		"saal", saaldaten.Saal, "saal_id", saalID,
		"plaetze", len(aufbau), "kameras", len(saaldaten.Kameras))

	steuerung := kamera.NeuViscaIP(konfiguration.KameraZeitlimit())
	sitzung, err := kern.Neu(saalID, aufbau, konfiguration.MaxOffeneMikrofone,
		konfiguration.KameraZeitlimit(), steuerung, ablage, protokoll)
	if err != nil {
		return err
	}

	dienst := &http.Server{
		Addr:              konfiguration.Adresse,
		Handler:           web.Neu(sitzung, webVerzeichnis, protokoll).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	fehler := make(chan error, 1)
	go func() {
		protokoll.Info("server hört",
			"adresse", konfiguration.Adresse, "max_offene_mikrofone", konfiguration.MaxOffeneMikrofone)
		if err := dienst.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fehler <- err
			return
		}
		fehler <- nil
	}()

	select {
	case err := <-fehler:
		return err
	case <-ctx.Done():
		protokoll.Info("server fährt herunter")
		aus, fertig := context.WithTimeout(context.Background(), 5*time.Second)
		defer fertig()
		return dienst.Shutdown(aus)
	}
}
