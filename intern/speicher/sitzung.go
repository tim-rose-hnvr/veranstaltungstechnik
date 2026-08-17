package speicher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"github.com/tim-rose-hnvr/kameranachverfolgung/intern/kern"
)

// Sitzungsdaten ist der Inhalt von sitzung.json.
type Sitzungsdaten struct {
	Titel      string           `json:"titel"`
	Teilnahmen []Teilnahmedaten `json:"teilnahmen"`
}

// Teilnahmedaten verbindet Person, Platz und Rolle.
type Teilnahmedaten struct {
	Platz  int    `json:"platz"`
	Person string `json:"person"`
	Rolle  string `json:"rolle"`
	Pin    string `json:"pin"`
}

// Sitzungsstand ist das Ergebnis des Imports.
type Sitzungsstand struct {
	SitzungID     string
	Titel         string
	Zustand       kern.Sitzungszustand
	Beginn        *time.Time // Nullpunkt der Zeitachse, solange nicht eröffnet: nil
	Teilnahmen    []kern.Teilnahmeaufbau
	Wortmeldungen []kern.Wortmeldung
}

// SitzungLesen liest die Sitzungsdatei und prüft sie.
func SitzungLesen(pfad string) (Sitzungsdaten, error) {
	roh, err := os.ReadFile(pfad)
	if err != nil {
		return Sitzungsdaten{}, fmt.Errorf("sitzungsdatei %s nicht lesbar: %w", pfad, err)
	}

	var d Sitzungsdaten
	entschluessler := json.NewDecoder(bytes.NewReader(roh))
	entschluessler.DisallowUnknownFields()
	if err := entschluessler.Decode(&d); err != nil {
		return Sitzungsdaten{}, fmt.Errorf("sitzungsdatei %s nicht lesbar: %w", pfad, err)
	}
	if err := d.Pruefen(); err != nil {
		return Sitzungsdaten{}, fmt.Errorf("sitzungsdatei %s: %w", pfad, err)
	}
	return d, nil
}

// Pruefen stellt sicher, dass die Sitzungsdatei in sich stimmig ist.
func (d Sitzungsdaten) Pruefen() error {
	if d.Titel == "" {
		return fmt.Errorf("feld \"titel\" fehlt")
	}
	if len(d.Teilnahmen) == 0 {
		return fmt.Errorf("keine teilnahme angegeben")
	}

	plaetze := make(map[int]bool, len(d.Teilnahmen))
	personen := make(map[string]bool, len(d.Teilnahmen))
	for _, t := range d.Teilnahmen {
		if t.Platz < 1 {
			return fmt.Errorf("teilnahme mit platz %d", t.Platz)
		}
		if plaetze[t.Platz] {
			return fmt.Errorf("platz %d kommt doppelt vor", t.Platz)
		}
		plaetze[t.Platz] = true

		if t.Person == "" {
			return fmt.Errorf("teilnahme auf platz %d ohne person", t.Platz)
		}
		if personen[t.Person] {
			return fmt.Errorf("person %q kommt doppelt vor", t.Person)
		}
		personen[t.Person] = true

		if _, err := kern.RolleLesen(t.Rolle); err != nil {
			return fmt.Errorf("platz %d: %w", t.Platz, err)
		}
		if len(t.Pin) != 4 {
			return fmt.Errorf("platz %d: die pin muss vierstellig sein", t.Platz)
		}
		for _, z := range t.Pin {
			if z < '0' || z > '9' {
				return fmt.Errorf("platz %d: die pin besteht nur aus ziffern", t.Platz)
			}
		}
	}
	return nil
}

// SitzungImportieren schreibt die Sitzungsdatei in die Tabellen. Wiederholtes
// Aufrufen mit derselben Datei erzeugt keine zusätzlichen Zeilen. Die PIN wird
// nur als bcrypt-Hash abgelegt und nur dann neu gesetzt, wenn sie sich
// geändert hat — sonst wäre der Import nicht wiederholbar.
func (p *Postgres) SitzungImportieren(ctx context.Context, saalID string, d Sitzungsdaten) (Sitzungsstand, error) {
	if err := d.Pruefen(); err != nil {
		return Sitzungsstand{}, err
	}
	var stand Sitzungsstand

	err := p.imVorgang(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", sperreImport); err != nil {
			return err
		}

		var organisationID string
		if err := tx.QueryRow(ctx,
			"SELECT organisation_id::text FROM saal WHERE id = $1", saalID).Scan(&organisationID); err != nil {
			return fmt.Errorf("saal %s: %w", saalID, err)
		}

		// Die Sitzung hat keine Eindeutigkeit auf (saal_id, titel), deshalb
		// erst suchen, dann anlegen — unter der Vorgangssperre von oben.
		var zustand string
		var beginn *time.Time
		err := tx.QueryRow(ctx, `
			SELECT id::text, zustand, beginn FROM sitzung
			WHERE saal_id = $1 AND titel = $2 ORDER BY id LIMIT 1`,
			saalID, d.Titel).Scan(&stand.SitzungID, &zustand, &beginn)
		switch {
		case err == nil:
			stand.Zustand = kern.Sitzungszustand(zustand)
			stand.Beginn = beginn
		case errors.Is(err, pgx.ErrNoRows):
			if err := tx.QueryRow(ctx, `
				INSERT INTO sitzung (saal_id, titel) VALUES ($1, $2)
				RETURNING id::text, zustand`, saalID, d.Titel).Scan(&stand.SitzungID, &zustand); err != nil {
				return fmt.Errorf("sitzung %q anlegen: %w", d.Titel, err)
			}
			stand.Zustand = kern.Sitzungszustand(zustand)
		default:
			return fmt.Errorf("sitzung %q suchen: %w", d.Titel, err)
		}
		stand.Titel = d.Titel

		for _, t := range d.Teilnahmen {
			var personID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO person (organisation_id, name) VALUES ($1, $2)
				ON CONFLICT (organisation_id, name) DO UPDATE SET name = EXCLUDED.name
				RETURNING id::text`, organisationID, t.Person).Scan(&personID); err != nil {
				return fmt.Errorf("person %q: %w", t.Person, err)
			}

			var platzID string
			if err := tx.QueryRow(ctx,
				"SELECT id::text FROM platz WHERE saal_id = $1 AND nummer = $2",
				saalID, t.Platz).Scan(&platzID); err != nil {
				return fmt.Errorf("platz %d gibt es im saal nicht: %w", t.Platz, err)
			}

			teilnahmeID, pinHash, err := p.teilnahmeSichern(ctx, tx, stand.SitzungID, personID, platzID, t)
			if err != nil {
				return err
			}
			stand.Teilnahmen = append(stand.Teilnahmen, kern.Teilnahmeaufbau{
				ID:          teilnahmeID,
				PlatzNummer: t.Platz,
				Person:      t.Person,
				Rolle:       kern.Rolle(t.Rolle),
				PinHash:     pinHash,
			})
		}
		return nil
	})
	if err != nil {
		return Sitzungsstand{}, err
	}

	stand.Wortmeldungen, err = p.OffeneWortmeldungen(ctx, stand.SitzungID)
	if err != nil {
		return Sitzungsstand{}, err
	}
	return stand, nil
}

// teilnahmeSichern legt die Teilnahme an oder gleicht sie ab.
func (p *Postgres) teilnahmeSichern(ctx context.Context, tx pgx.Tx,
	sitzungID, personID, platzID string, t Teilnahmedaten) (string, []byte, error) {

	var id string
	var vorhandenerHash []byte
	err := tx.QueryRow(ctx, `
		SELECT id::text, pin_hash FROM teilnahme
		WHERE sitzung_id = $1 AND platz_id = $2`, sitzungID, platzID).Scan(&id, &vorhandenerHash)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		hash, err := bcrypt.GenerateFromPassword([]byte(t.Pin), bcrypt.DefaultCost)
		if err != nil {
			return "", nil, fmt.Errorf("pin für platz %d: %w", t.Platz, err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO teilnahme (sitzung_id, person_id, platz_id, rolle, pin_hash)
			VALUES ($1, $2, $3, $4, $5) RETURNING id::text`,
			sitzungID, personID, platzID, t.Rolle, hash).Scan(&id); err != nil {
			return "", nil, fmt.Errorf("teilnahme auf platz %d: %w", t.Platz, err)
		}
		return id, hash, nil

	case err != nil:
		return "", nil, fmt.Errorf("teilnahme auf platz %d suchen: %w", t.Platz, err)
	}

	// Vorhanden: Rolle und Person abgleichen, die PIN nur bei Änderung neu
	// setzen. Ein bcrypt-Hash ist bei jedem Aufruf anders — würde er blind
	// überschrieben, wäre der Import nicht mehr wiederholbar.
	hash := vorhandenerHash
	if bcrypt.CompareHashAndPassword(vorhandenerHash, []byte(t.Pin)) != nil {
		neu, err := bcrypt.GenerateFromPassword([]byte(t.Pin), bcrypt.DefaultCost)
		if err != nil {
			return "", nil, fmt.Errorf("pin für platz %d: %w", t.Platz, err)
		}
		hash = neu
	}
	if _, err := tx.Exec(ctx, `
		UPDATE teilnahme SET person_id = $2, rolle = $3, pin_hash = $4 WHERE id = $1`,
		id, personID, t.Rolle, hash); err != nil {
		return "", nil, fmt.Errorf("teilnahme auf platz %d abgleichen: %w", t.Platz, err)
	}
	return id, hash, nil
}

// SitzungZustandSetzen hält den Zustandswechsel der Sitzung fest. zeit kommt
// vom Kern, damit der Nullpunkt der Zeitachse in Datenbank und Kern derselbe
// ist — sonst laufen Ereigniszeiten und Aufzeichnung auseinander.
func (p *Postgres) SitzungZustandSetzen(ctx context.Context, sitzungID string,
	zustand kern.Sitzungszustand, zeit time.Time) error {

	var err error
	switch zustand {
	case kern.SitzungLaufend:
		_, err = p.teich.Exec(ctx, `
			UPDATE sitzung SET zustand = $2, beginn = coalesce(beginn, $3) WHERE id = $1`,
			sitzungID, string(zustand), zeit)
	case kern.SitzungGeschlossen, kern.SitzungArchiviert:
		_, err = p.teich.Exec(ctx, `
			UPDATE sitzung SET zustand = $2, ende = $3 WHERE id = $1`, sitzungID, string(zustand), zeit)
	default:
		_, err = p.teich.Exec(ctx, "UPDATE sitzung SET zustand = $2 WHERE id = $1", sitzungID, string(zustand))
	}
	if err != nil {
		return fmt.Errorf("sitzungszustand setzen: %w", err)
	}
	return nil
}

// TeilnahmeZustandSetzen hält Kommen und Gehen fest.
func (p *Postgres) TeilnahmeZustandSetzen(ctx context.Context, teilnahmeID string, zustand kern.Teilnahmezustand) error {
	if _, err := p.teich.Exec(ctx,
		"UPDATE teilnahme SET zustand = $2 WHERE id = $1", teilnahmeID, string(zustand)); err != nil {
		return fmt.Errorf("teilnahmezustand setzen: %w", err)
	}
	return nil
}

// WortmeldungAnlegen hängt eine Meldung an die Redeliste. folge_nr bestimmt
// die Reihenfolge und bleibt lückenlos.
func (p *Postgres) WortmeldungAnlegen(ctx context.Context, sitzungID, teilnahmeID string) (string, int64, error) {
	for versuch := 0; versuch < 5; versuch++ {
		var id string
		var folgeNr int64
		err := p.teich.QueryRow(ctx, `
			INSERT INTO wortmeldung (sitzung_id, teilnahme_id, folge_nr)
			SELECT $1, $2, coalesce(max(folge_nr), 0) + 1 FROM wortmeldung WHERE sitzung_id = $1
			RETURNING id::text, folge_nr`, sitzungID, teilnahmeID).Scan(&id, &folgeNr)
		if err == nil {
			return id, folgeNr, nil
		}
		var pgFehler *pgconn.PgError
		if errors.As(err, &pgFehler) && pgFehler.Code == "23505" {
			continue // gleichzeitig gemeldet, neue Nummer holen
		}
		return "", 0, fmt.Errorf("wortmeldung anlegen: %w", err)
	}
	return "", 0, fmt.Errorf("wortmeldung anlegen: nummer blieb belegt")
}

// WortmeldungZustandSetzen schreibt den Übergang fest.
func (p *Postgres) WortmeldungZustandSetzen(ctx context.Context, wortmeldungID string, zustand kern.Wortzustand) error {
	var err error
	switch zustand {
	case kern.WortErteilt:
		_, err = p.teich.Exec(ctx, `
			UPDATE wortmeldung SET zustand = $2, erteilt = coalesce(erteilt, now()) WHERE id = $1`,
			wortmeldungID, string(zustand))
	case kern.WortBeendet, kern.WortEntzogen, kern.WortZurueckgezogen:
		_, err = p.teich.Exec(ctx, `
			UPDATE wortmeldung SET zustand = $2, beendet = now() WHERE id = $1`,
			wortmeldungID, string(zustand))
	default:
		_, err = p.teich.Exec(ctx,
			"UPDATE wortmeldung SET zustand = $2 WHERE id = $1", wortmeldungID, string(zustand))
	}
	if err != nil {
		return fmt.Errorf("wortmeldung setzen: %w", err)
	}
	return nil
}

// OffeneWortmeldungen liefert die Redeliste einer Sitzung.
func (p *Postgres) OffeneWortmeldungen(ctx context.Context, sitzungID string) ([]kern.Wortmeldung, error) {
	zeilen, err := p.teich.Query(ctx, `
		SELECT w.id::text, w.folge_nr, pl.nummer, w.zustand
		FROM wortmeldung w
		JOIN teilnahme t ON t.id = w.teilnahme_id
		JOIN platz pl ON pl.id = t.platz_id
		WHERE w.sitzung_id = $1 AND w.zustand IN ('gemeldet','erteilt','laufend')
		ORDER BY w.folge_nr`, sitzungID)
	if err != nil {
		return nil, fmt.Errorf("redeliste lesen: %w", err)
	}
	defer zeilen.Close()

	var liste []kern.Wortmeldung
	for zeilen.Next() {
		var w kern.Wortmeldung
		var zustand string
		if err := zeilen.Scan(&w.ID, &w.FolgeNr, &w.PlatzNummer, &zustand); err != nil {
			return nil, fmt.Errorf("wortmeldung lesen: %w", err)
		}
		if w.Zustand, err = kern.WortzustandLesen(zustand); err != nil {
			return nil, err
		}
		liste = append(liste, w)
	}
	return liste, zeilen.Err()
}

// LeitungAusKette liest den zuletzt übergebenen Staffelstab aus dem
// Ereignisprotokoll. 0 bedeutet: noch nie übergeben.
func (p *Postgres) LeitungAusKette(ctx context.Context, saalID string) (int, error) {
	var platz *int
	err := p.teich.QueryRow(ctx, `
		SELECT (nutzlast->>'an')::int FROM ereignis
		WHERE saal_id = $1 AND art = 'leitung_uebergeben'
		ORDER BY folge_nr DESC LIMIT 1`, saalID).Scan(&platz)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("leitung aus der kette lesen: %w", err)
	case platz == nil:
		return 0, nil
	}
	return *platz, nil
}
