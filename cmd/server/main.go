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

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kamera"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	protokollpaket "github.com/tim-rose-hnvr/veranstaltungstechnik/intern/protokoll"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/vorabcheck"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/web"
)

// Migrationen in dieser Reihenfolge. Die Wächtertabelle ist die letzte, die
// die jeweilige Datei anlegt — gibt es sie, ist die Migration gelaufen.
var migrationen = []struct{ Datei, Waechter string }{
	{"migrationen/001_grundlage.sql", "ereignis"},
	{"migrationen/002_sitzung.sql", "wortmeldung"},
	{"migrationen/003_abstimmung.sql", "stimmabgabe"},
}

const webVerzeichnis = "web"

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

// VorfuehrBetrieb steht in config.yaml statt einer Verbindungszeichenkette und
// hält alles im Arbeitsspeicher. Damit läuft das System ohne jede Einrichtung
// — aber nach dem Beenden ist nichts mehr da.
const VorfuehrBetrieb = "gedaechtnis"

// ablageOeffnen wählt zwischen PostgreSQL und dem Vorführbetrieb.
func ablageOeffnen(ctx context.Context, datenbank string, protokoll *slog.Logger) (speicher.Ablage, error) {
	if datenbank == VorfuehrBetrieb {
		protokoll.Warn("vorführbetrieb: alles liegt im arbeitsspeicher und ist nach dem beenden weg")
		return speicher.NeuGedaechtnis(), nil
	}

	pg, err := speicher.Verbinden(ctx, datenbank)
	if err != nil {
		return nil, err
	}
	for _, m := range migrationen {
		migration, err := os.ReadFile(m.Datei)
		if err != nil {
			pg.Schliessen()
			return nil, err
		}
		if err := pg.SchemaSicherstellen(ctx, m.Waechter, string(migration)); err != nil {
			pg.Schliessen()
			return nil, err
		}
	}
	return pg, nil
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
	sitzungsdaten, err := speicher.SitzungLesen(konfiguration.SitzungsDatei)
	if err != nil {
		return err
	}

	anlaufCtx, anlaufFertig := context.WithTimeout(ctx, 30*time.Second)
	defer anlaufFertig()

	ablage, err := ablageOeffnen(anlaufCtx, konfiguration.Datenbank, protokoll)
	if err != nil {
		return err
	}
	if schliessbar, passt := ablage.(interface{ Schliessen() }); passt {
		defer schliessbar.Schliessen()
	}

	saalID, plaetze, err := ablage.SaalImportieren(anlaufCtx, saaldaten)
	if err != nil {
		return err
	}
	protokoll.Info("saal eingelesen",
		"saal", saaldaten.Saal, "saal_id", saalID,
		"plaetze", len(plaetze), "kameras", len(saaldaten.Kameras))

	stand, err := ablage.SitzungImportieren(anlaufCtx, saalID, sitzungsdaten)
	if err != nil {
		return err
	}
	leitungPlatz, err := ablage.LeitungAusKette(anlaufCtx, saalID)
	if err != nil {
		return err
	}
	abstimmung, err := ablage.LetzteAbstimmung(anlaufCtx, stand.SitzungID)
	if err != nil {
		return err
	}
	protokoll.Info("sitzung eingelesen",
		"titel", stand.Titel, "zustand", stand.Zustand,
		"teilnahmen", len(stand.Teilnahmen), "redeliste", len(stand.Wortmeldungen))

	steuerung := kamera.NeuViscaIP(konfiguration.KameraZeitlimit())
	aufbau := kern.Aufbau{
		SaalID:         saalID,
		SitzungID:      stand.SitzungID,
		Titel:          stand.Titel,
		SitzungZustand: stand.Zustand,
		Beginn:         stand.Beginn,
		Plaetze:        plaetze,
		Teilnahmen:     stand.Teilnahmen,
		Wortmeldungen:  stand.Wortmeldungen,
		Abstimmung:     abstimmung,
		LeitungPlatz:   leitungPlatz,
		MaxOffen:       konfiguration.MaxOffeneMikrofone,
		Zeitlimit:      konfiguration.KameraZeitlimit(),
	}
	sitzung, err := kern.Neu(aufbau, steuerung, ablage, protokoll)
	if err != nil {
		return err
	}

	oberflaeche := web.Neu(sitzung, webVerzeichnis, protokoll)
	oberflaeche.SetzeProtokoll(
		protokollpaket.Neu(saalID, saaldaten.Saal, ablage, stand.Teilnahmen), stand.SitzungID)
	oberflaeche.SetzeVorabcheck(
		vorabcheck.Neu(aufbau, sitzung, steuerung, ablage, konfiguration.KameraZeitlimit()))

	dienst := &http.Server{
		Addr:              konfiguration.Adresse,
		Handler:           oberflaeche.Handler(),
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
