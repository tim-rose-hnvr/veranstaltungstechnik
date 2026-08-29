// Befehl server startet den Sitzungsserver: Saal einlesen, Zustand halten,
// Clients bedienen, Kamera fahren.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kamera"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	protokollpaket "github.com/tim-rose-hnvr/veranstaltungstechnik/intern/protokoll"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/siegel"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/vorabcheck"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/web"
)

const (
	webVerzeichnis        = "web"
	migrationsVerzeichnis = "migrationen"
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
	for _, m := range speicher.Migrationen {
		migration, err := os.ReadFile(filepath.Join(migrationsVerzeichnis, m.Datei))
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
		"teilnahmen", len(stand.Teilnahmen), "tagesordnung", len(stand.Tagesordnung),
		"unterlagen", len(stand.Unterlagen), "redeliste", len(stand.Wortmeldungen))

	// Simulierte Kameras hören auf denselben Adressen wie die echten. Die
	// Steuerung merkt keinen Unterschied — sie schickt UDP und wartet auf die
	// Quittung.
	var attrappe *kamera.Attrappe
	if konfiguration.KameraAttrappe {
		attrappe = kamera.NeuAttrappe(protokoll)
		defer attrappe.Schliessen()
		for _, k := range saaldaten.Kameras {
			adresse, err := attrappe.Anschalten(k.Name, k.Adresse)
			if err != nil {
				return fmt.Errorf("kamera-attrappe: %w", err)
			}
			protokoll.Warn("simulierte kamera hört", "kamera", k.Name, "adresse", adresse)
		}
	}

	steuerung := kamera.NeuViscaIP(konfiguration.KameraZeitlimit())
	aufbau := kern.Aufbau{
		SaalID:         saalID,
		Saal:           saaldaten.Saal,
		SitzungID:      stand.SitzungID,
		Titel:          stand.Titel,
		SitzungZustand: stand.Zustand,
		Beginn:         stand.Beginn,
		Plaetze:        plaetze,
		Teilnahmen:     stand.Teilnahmen,
		Tagesordnung:   stand.Tagesordnung,
		Unterlagen:     stand.Unterlagen,
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

	// Die Kette wird abgeschlossen und unterschrieben: sie allein zeigt nur,
	// dass niemand einen Eintrag geändert hat — nicht, dass niemand die ganze
	// Kette neu gerechnet hat.
	schluessel, err := siegel.Laden(konfiguration.SiegelSchluessel)
	if err != nil {
		return err
	}
	siegler := siegel.Neu(saalID, schluessel, ablage, protokoll)
	protokoll.Info("siegelschlüssel bereit",
		"datei", konfiguration.SiegelSchluessel,
		"fingerabdruck", siegel.Fingerabdruck(schluessel.Oeffentlich))
	if konfiguration.SiegelUhrzeit != "" {
		if err := siegler.Taeglich(ctx, konfiguration.SiegelUhrzeit); err != nil {
			return err
		}
		protokoll.Info("täglicher kettenabschluss eingerichtet", "uhrzeit", konfiguration.SiegelUhrzeit)
	}

	oberflaeche := web.Neu(sitzung, webVerzeichnis, protokoll)
	oberflaeche.SetzeSiegler(siegler, schluessel.Oeffentlich, saalID, ablage)
	oberflaeche.SetzeProtokoll(
		protokollpaket.Neu(saalID, saaldaten.Saal, ablage, stand.Teilnahmen), stand.SitzungID)
	pruefer := vorabcheck.Neu(aufbau, sitzung, steuerung, ablage, konfiguration.KameraZeitlimit())
	pruefer.SetzeSiegelschluessel(schluessel.Oeffentlich)
	oberflaeche.SetzeVorabcheck(pruefer)

	if konfiguration.Emulator {
		protokoll.Warn("prüfstelle unter /emulator freigeschaltet — sie gibt die PINs preis")
		kameras := make([]web.Kameraangabe, 0, len(saaldaten.Kameras))
		for _, k := range saaldaten.Kameras {
			kameras = append(kameras, web.Kameraangabe{Name: k.Name, Adresse: k.Adresse, Kanal: k.Kanal})
		}
		pins := make(map[int]string, len(sitzungsdaten.Teilnahmen))
		for _, t := range sitzungsdaten.Teilnahmen {
			pins[t.Platz] = t.Pin
		}
		oberflaeche.SetzeEmulator(web.Emulatordaten{
			Saal: saaldaten.Saal, SaalID: saalID, Plaetze: plaetze,
			Pins: pins, Kameras: kameras, Attrappe: attrappe, Kette: ablage,
		})
	}

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
		aus, fertig := context.WithTimeout(context.Background(), 10*time.Second)
		defer fertig()
		// Ein Server, der ohne Siegel stoppt, hinterlässt eine ungeschlossene
		// Kette. Das Siegel kommt vor dem Herunterfahren, nicht danach.
		if abschluss, err := siegler.Siegeln(aus); err != nil {
			protokoll.Error("abschlusssiegel nicht gesetzt", "grund", err)
		} else if abschluss.Neu {
			protokoll.Info("kette abgeschlossen", "von", abschluss.Von, "bis", abschluss.Bis)
		}
		return dienst.Shutdown(aus)
	}
}
