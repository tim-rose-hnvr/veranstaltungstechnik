package speicher

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
)

// StandardOrganisation ist der Mandant, unter dem ein importierter Saal
// angelegt wird. Meilenstein 1 kennt nur einen — die Spalte organisation_id
// ist trotzdem von Anfang an in jeder Tabelle.
const StandardOrganisation = "Standard"

// sperreImport serialisiert den Saal-Import über alle Serverinstanzen hinweg.
// Beliebige, aber feste Zahl für pg_advisory_xact_lock.
const sperreImport int64 = 8_150_233_001

// Postgres ist die Ablage in einer PostgreSQL-Datenbank.
type Postgres struct {
	teich *pgxpool.Pool
}

// Verbinden öffnet den Verbindungspool und prüft die Verbindung.
func Verbinden(ctx context.Context, dsn string) (*Postgres, error) {
	teich, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("datenbank: %w", err)
	}
	if err := teich.Ping(ctx); err != nil {
		teich.Close()
		return nil, fmt.Errorf("datenbank nicht erreichbar: %w", err)
	}
	return &Postgres{teich: teich}, nil
}

// Schliessen gibt den Verbindungspool frei.
func (p *Postgres) Schliessen() { p.teich.Close() }

// SchemaSicherstellen spielt eine Migration ein, wenn sie noch nicht gelaufen
// ist. Ob sie gelaufen ist, verrät die Wächtertabelle — die letzte Tabelle,
// die die Migration anlegt. So braucht es keine eigene Verwaltungstabelle.
func (p *Postgres) SchemaSicherstellen(ctx context.Context, waechter, migration string) error {
	return p.imVorgang(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", sperreImport); err != nil {
			return err
		}
		var vorhanden *string
		if err := tx.QueryRow(ctx, "SELECT to_regclass($1)::text", "public."+waechter).Scan(&vorhanden); err != nil {
			return fmt.Errorf("schema prüfen: %w", err)
		}
		if vorhanden != nil {
			return nil
		}
		if _, err := tx.Exec(ctx, migration); err != nil {
			return fmt.Errorf("migration %s einspielen: %w", waechter, err)
		}
		return nil
	})
}

// SaalImportieren schreibt die Saaldatei in die Tabellen. Wiederholtes
// Aufrufen mit derselben Datei erzeugt keine zusätzlichen Zeilen.
func (p *Postgres) SaalImportieren(ctx context.Context, d Saaldaten) (string, []kern.Platzaufbau, error) {
	if err := d.Pruefen(); err != nil {
		return "", nil, err
	}

	var saalID string
	var aufbau []kern.Platzaufbau

	err := p.imVorgang(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", sperreImport); err != nil {
			return err
		}

		organisationID, err := p.organisation(ctx, tx, StandardOrganisation)
		if err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `
			INSERT INTO saal (organisation_id, name) VALUES ($1, $2)
			ON CONFLICT (organisation_id, name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, organisationID, d.Saal).Scan(&saalID); err != nil {
			return fmt.Errorf("saal %q: %w", d.Saal, err)
		}

		kameraIDs := make(map[string]string, len(d.Kameras))
		kanaele := make(map[string]uint8, len(d.Kameras))
		adressen := make(map[string]string, len(d.Kameras))
		for _, k := range d.Kameras {
			var id string
			if err := tx.QueryRow(ctx, `
				INSERT INTO kamera (saal_id, name, adresse, kanal) VALUES ($1, $2, $3, $4)
				ON CONFLICT (saal_id, name)
				DO UPDATE SET adresse = EXCLUDED.adresse, kanal = EXCLUDED.kanal
				RETURNING id`, saalID, k.Name, k.Adresse, int16(k.Kanal)).Scan(&id); err != nil {
				return fmt.Errorf("kamera %q: %w", k.Name, err)
			}
			kameraIDs[k.Name] = id
			kanaele[k.Name] = k.Kanal
			adressen[k.Name] = k.Adresse
		}

		aufbau = make([]kern.Platzaufbau, 0, len(d.Plaetze))
		for _, pl := range d.Plaetze {
			var platzID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO platz (saal_id, nummer, name) VALUES ($1, $2, $3)
				ON CONFLICT (saal_id, nummer) DO UPDATE SET name = EXCLUDED.name
				RETURNING id`, saalID, pl.Nummer, pl.Name).Scan(&platzID); err != nil {
				return fmt.Errorf("platz %d: %w", pl.Nummer, err)
			}

			kameraID := kameraIDs[pl.Kamera]
			if _, err := tx.Exec(ctx, `
				INSERT INTO preset (kamera_id, platz_id, nummer) VALUES ($1, $2, $3)
				ON CONFLICT (kamera_id, platz_id) DO UPDATE SET nummer = EXCLUDED.nummer`,
				kameraID, platzID, int16(pl.Preset)); err != nil {
				return fmt.Errorf("preset für platz %d: %w", pl.Nummer, err)
			}

			aufbau = append(aufbau, kern.Platzaufbau{
				Nummer:        pl.Nummer,
				Name:          pl.Name,
				KameraName:    pl.Kamera,
				KameraAdresse: adressen[pl.Kamera],
				Kanal:         kanaele[pl.Kamera],
				Preset:        pl.Preset,
			})
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	return saalID, aufbau, nil
}

// organisation legt den Mandanten an oder findet ihn. Die Tabelle hat keine
// Eindeutigkeit auf name, deshalb erst suchen, dann anlegen — unter der
// Vorgangssperre von oben.
func (p *Postgres) organisation(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, "SELECT id::text FROM organisation WHERE name = $1 ORDER BY id LIMIT 1", name).Scan(&id)
	switch {
	case err == nil:
		return id, nil
	case errors.Is(err, pgx.ErrNoRows):
		if err := tx.QueryRow(ctx, "INSERT INTO organisation (name) VALUES ($1) RETURNING id", name).Scan(&id); err != nil {
			return "", fmt.Errorf("organisation %q anlegen: %w", name, err)
		}
		return id, nil
	default:
		return "", fmt.Errorf("organisation %q suchen: %w", name, err)
	}
}

// EreignisAnfuegen hängt ein Ereignis an die Kette des Saals an. Der letzte
// Eintrag wird dabei gesperrt, damit die Kette lückenlos bleibt.
func (p *Postgres) EreignisAnfuegen(ctx context.Context, saalID, art string, nutzlast map[string]any) (kern.Ereignis, error) {
	roh, err := kern.Kanonisch(nutzlast)
	if err != nil {
		return kern.Ereignis{}, err
	}

	// Ist der Saal noch ohne Ereignis, gibt es keine Zeile zum Sperren. Dann
	// entscheidet die Eindeutigkeit auf (saal_id, folge_nr) — der unterlegene
	// Vorgang versucht es erneut.
	for versuch := 0; versuch < 5; versuch++ {
		var e kern.Ereignis
		err := p.imVorgang(ctx, func(tx pgx.Tx) error {
			var vorher *kern.Ereignis
			var folgeNr int64
			var hash []byte
			zeile := tx.QueryRow(ctx, `
				SELECT folge_nr, hash FROM ereignis
				WHERE saal_id = $1 ORDER BY folge_nr DESC LIMIT 1 FOR UPDATE`, saalID)
			switch err := zeile.Scan(&folgeNr, &hash); {
			case err == nil:
				vorher = &kern.Ereignis{FolgeNr: folgeNr, Hash: hash}
			case errors.Is(err, pgx.ErrNoRows):
			default:
				return fmt.Errorf("letztes ereignis lesen: %w", err)
			}

			neu, err := kern.NaechstesEreignis(vorher, time.Now(), art, nutzlast)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO ereignis (saal_id, folge_nr, zeit, art, nutzlast, vorgaenger_hash, hash)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				saalID, neu.FolgeNr, neu.Zeit, neu.Art, roh, neu.VorgaengerHash, neu.Hash); err != nil {
				return err
			}
			e = neu
			return nil
		})
		if err == nil {
			return e, nil
		}
		var pgFehler *pgconn.PgError
		if errors.As(err, &pgFehler) && pgFehler.Code == "23505" {
			continue // Kette wurde gleichzeitig verlängert, neu aufsetzen
		}
		return kern.Ereignis{}, fmt.Errorf("ereignis %q anfügen: %w", art, err)
	}
	return kern.Ereignis{}, fmt.Errorf("ereignis %q anfügen: kette blieb belegt", art)
}

// Ereignisse liefert die vollständige Kette eines Saals, aufsteigend.
func (p *Postgres) Ereignisse(ctx context.Context, saalID string) ([]kern.Ereignis, error) {
	zeilen, err := p.teich.Query(ctx, `
		SELECT folge_nr, zeit, art, nutzlast, vorgaenger_hash, hash
		FROM ereignis WHERE saal_id = $1 ORDER BY folge_nr`, saalID)
	if err != nil {
		return nil, fmt.Errorf("ereignisse lesen: %w", err)
	}
	defer zeilen.Close()

	var kette []kern.Ereignis
	for zeilen.Next() {
		var e kern.Ereignis
		if err := zeilen.Scan(&e.FolgeNr, &e.Zeit, &e.Art, &e.Nutzlast, &e.VorgaengerHash, &e.Hash); err != nil {
			return nil, fmt.Errorf("ereignis lesen: %w", err)
		}
		e.Zeit = kern.ZeitStutzen(e.Zeit)
		kette = append(kette, e)
	}
	return kette, zeilen.Err()
}

// Zaehlen liefert die Zeilenzahl einer Tabelle. Für den Import-Test.
func (p *Postgres) Zaehlen(ctx context.Context, tabelle string) (int, error) {
	erlaubt := map[string]bool{
		"organisation": true, "saal": true, "kamera": true,
		"platz": true, "preset": true, "ereignis": true,
		"person": true, "sitzung": true, "teilnahme": true, "wortmeldung": true,
		"abstimmung": true, "stimme": true, "stimmabgabe": true,
		"tagesordnungspunkt": true, "unterlage": true,
	}
	if !erlaubt[tabelle] {
		return 0, fmt.Errorf("unbekannte tabelle %q", tabelle)
	}
	var anzahl int
	if err := p.teich.QueryRow(ctx, "SELECT count(*) FROM "+tabelle).Scan(&anzahl); err != nil {
		return 0, err
	}
	return anzahl, nil
}

func (p *Postgres) imVorgang(ctx context.Context, arbeit func(pgx.Tx) error) error {
	tx, err := p.teich.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck // nach Commit wirkungslos

	if err := arbeit(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Migrationen sind die Schemadateien in der Reihenfolge, in der sie laufen.
// Waechter ist die letzte Tabelle, die die jeweilige Datei anlegt — gibt es
// sie, ist die Migration gelaufen. Die Liste steht hier und nicht im Server,
// damit Betrieb und Test nicht auseinanderlaufen können.
var Migrationen = []struct{ Datei, Waechter string }{
	{"001_grundlage.sql", "ereignis"},
	{"002_sitzung.sql", "wortmeldung"},
	{"003_abstimmung.sql", "stimmabgabe"},
	{"004_tagesordnung.sql", "tagesordnungspunkt"},
	{"005_unterlage.sql", "unterlage"},
}
