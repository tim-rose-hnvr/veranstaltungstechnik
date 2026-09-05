package speicher

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
)

// AbstimmungAnlegen legt eine Abstimmung an. folge_nr bestimmt die Reihenfolge
// innerhalb der Sitzung.
func (p *Postgres) AbstimmungAnlegen(ctx context.Context, sitzungID, titel string,
	art kern.Abstimmungsart) (string, int64, error) {

	for versuch := 0; versuch < 5; versuch++ {
		var id string
		var folgeNr int64
		// Abstimmung und Zähler in EINEM Vorgang: eine geheime Abstimmung ohne
		// ihre Zähler nähme keine Stimme mehr an. Lieber beides oder nichts.
		err := p.imVorgang(ctx, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, `
				INSERT INTO abstimmung (sitzung_id, folge_nr, titel, art)
				SELECT $1, coalesce(max(folge_nr), 0) + 1, $2, $3
				FROM abstimmung WHERE sitzung_id = $1
				RETURNING id::text, folge_nr`, sitzungID, titel, string(art)).Scan(&id, &folgeNr); err != nil {
				return err
			}
			// Bei geheimer Wahl müssen die drei Zähler vor der ersten Stimme
			// dastehen, denn jede Stimmabgabe rührt alle drei an — nur so
			// verrät xmin nicht, welche gewählt wurde.
			if art != kern.AbstimmungGeheim {
				return nil
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO stimme_zaehler (abstimmung_id, wahl)
				VALUES ($1,'ja'), ($1,'nein'), ($1,'enthaltung')`, id)
			return err
		})
		if err == nil {
			return id, folgeNr, nil
		}
		var pgFehler *pgconn.PgError
		if errors.As(err, &pgFehler) && pgFehler.Code == "23505" {
			continue // gleichzeitig angelegt, neue Nummer holen
		}
		return "", 0, fmt.Errorf("abstimmung anlegen: %w", err)
	}
	return "", 0, fmt.Errorf("abstimmung anlegen: nummer blieb belegt")
}

// AbstimmungStarten friert Beschlussfähigkeit und Quorum ein.
func (p *Postgres) AbstimmungStarten(ctx context.Context, abstimmungID string,
	stimmberechtigt, anwesend, quorum int, zeit time.Time) error {

	if _, err := p.teich.Exec(ctx, `
		UPDATE abstimmung SET zustand = 'laufend', stimmberechtigt = $2,
		       anwesend = $3, quorum = $4, beginn = $5 WHERE id = $1`,
		abstimmungID, stimmberechtigt, anwesend, quorum, zeit); err != nil {
		return fmt.Errorf("abstimmung starten: %w", err)
	}
	return nil
}

// AbstimmungZustandSetzen schreibt den Übergang fest.
func (p *Postgres) AbstimmungZustandSetzen(ctx context.Context, abstimmungID string,
	zustand kern.Abstimmungszustand, zeit time.Time) error {

	var err error
	switch zustand {
	case kern.AbstimmungAusgezaehlt, kern.AbstimmungFestgestellt, kern.AbstimmungAbgebrochen:
		_, err = p.teich.Exec(ctx,
			"UPDATE abstimmung SET zustand = $2, ende = coalesce(ende, $3) WHERE id = $1",
			abstimmungID, string(zustand), zeit)
	default:
		_, err = p.teich.Exec(ctx,
			"UPDATE abstimmung SET zustand = $2 WHERE id = $1", abstimmungID, string(zustand))
	}
	if err != nil {
		return fmt.Errorf("abstimmungszustand setzen: %w", err)
	}
	return nil
}

// StimmeAbgeben hält die Stimme fest. Bei geheimer Wahl bleibt die Zuordnung
// zur Person leer — festgehalten wird nur, dass abgestimmt wurde. Beides
// geschieht in einer Transaktion, sonst könnte eine Stimme ohne Sperre oder
// eine Sperre ohne Stimme entstehen.
func (p *Postgres) StimmeAbgeben(ctx context.Context, abstimmungID, teilnahmeID string,
	wahl kern.Wahl, geheim bool) error {

	return p.imVorgang(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"INSERT INTO stimmabgabe (abstimmung_id, teilnahme_id) VALUES ($1, $2)",
			abstimmungID, teilnahmeID); err != nil {
			var pgFehler *pgconn.PgError
			if errors.As(err, &pgFehler) && pgFehler.Code == "23505" {
				return fmt.Errorf("für diese teilnahme liegt bereits eine stimme vor")
			}
			return fmt.Errorf("stimmabgabe festhalten: %w", err)
		}

		if geheim {
			// Keine Einzelzeile. Alle drei Zähler werden angefasst, zwei davon
			// mit einem Zuschlag von null: damit tragen sie dasselbe xmin wie
			// die Stimmabgabe, aber xmin sagt nur noch, DASS abgestimmt wurde.
			// Warum das nötig ist, steht ausführlich in migrationen/007.
			marke, err := tx.Exec(ctx, `
				UPDATE stimme_zaehler
				SET anzahl = anzahl + (CASE WHEN wahl = $2 THEN 1 ELSE 0 END)
				WHERE abstimmung_id = $1`, abstimmungID, string(wahl))
			if err != nil {
				return fmt.Errorf("stimme zählen: %w", err)
			}
			if marke.RowsAffected() != 3 {
				// Fehlt ein Zähler, würde still falsch gezählt. Lieber die
				// Stimme abweisen als sie unbemerkt verlieren.
				return fmt.Errorf("stimme zählen: %d von 3 zählern gefunden", marke.RowsAffected())
			}
			return nil
		}
		_, err := tx.Exec(ctx,
			"INSERT INTO stimme (abstimmung_id, teilnahme_id, wahl) VALUES ($1, $2, $3)",
			abstimmungID, teilnahmeID, string(wahl))
		return err
	})
}

// LetzteAbstimmung lädt die jüngste Abstimmung einer Sitzung samt Zählung,
// damit ein Neustart mitten im Vorgang nichts verliert.
func (p *Postgres) LetzteAbstimmung(ctx context.Context, sitzungID string) (*kern.Abstimmung, error) {
	var a kern.Abstimmung
	var art, zustand string

	err := p.teich.QueryRow(ctx, `
		SELECT id::text, folge_nr, titel, art, zustand, stimmberechtigt, anwesend, quorum
		FROM abstimmung WHERE sitzung_id = $1 ORDER BY folge_nr DESC LIMIT 1`, sitzungID).
		Scan(&a.ID, &a.FolgeNr, &a.Titel, &art, &zustand, &a.Stimmberechtigt, &a.Anwesend, &a.Quorum)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("abstimmung lesen: %w", err)
	}
	if a.Art, err = kern.ArtLesen(art); err != nil {
		return nil, err
	}
	a.Zustand = kern.Abstimmungszustand(zustand)
	a.Abgegeben = map[int]bool{}
	a.Namentlich = map[int]kern.Wahl{}

	// Wer abgestimmt hat — ohne zu wissen, wie.
	zeilen, err := p.teich.Query(ctx, `
		SELECT pl.nummer FROM stimmabgabe s
		JOIN teilnahme t ON t.id = s.teilnahme_id
		JOIN platz pl ON pl.id = t.platz_id
		WHERE s.abstimmung_id = $1`, a.ID)
	if err != nil {
		return nil, fmt.Errorf("stimmabgaben lesen: %w", err)
	}
	for zeilen.Next() {
		var platz int
		if err := zeilen.Scan(&platz); err != nil {
			zeilen.Close()
			return nil, err
		}
		a.Abgegeben[platz] = true
	}
	zeilen.Close()
	if err := zeilen.Err(); err != nil {
		return nil, err
	}

	// Bei geheimer Wahl gibt es keine Einzelstimmen, nur Zähler. Genau das
	// ist der Punkt: es existiert nichts, was sich einer Person zuordnen
	// ließe. Siehe migrationen/007_geheime_wahl.sql.
	if a.Art == kern.AbstimmungGeheim {
		zaehler, err := p.teich.Query(ctx,
			"SELECT wahl, anzahl FROM stimme_zaehler WHERE abstimmung_id = $1", a.ID)
		if err != nil {
			return nil, fmt.Errorf("zähler lesen: %w", err)
		}
		defer zaehler.Close()
		for zaehler.Next() {
			var rohWahl string
			var anzahl int
			if err := zaehler.Scan(&rohWahl, &anzahl); err != nil {
				return nil, err
			}
			wahl, err := kern.WahlLesen(rohWahl)
			if err != nil {
				return nil, err
			}
			switch wahl {
			case kern.WahlJa:
				a.Ja = anzahl
			case kern.WahlNein:
				a.Nein = anzahl
			case kern.WahlEnthaltung:
				a.Enthaltung = anzahl
			}
		}
		return &a, zaehler.Err()
	}

	// Offen und namentlich: hier ist die Zuordnung gewollt und nachweispflichtig.
	stimmen, err := p.teich.Query(ctx, `
		SELECT s.wahl, pl.nummer
		FROM stimme s
		LEFT JOIN teilnahme t ON t.id = s.teilnahme_id
		LEFT JOIN platz pl ON pl.id = t.platz_id
		WHERE s.abstimmung_id = $1`, a.ID)
	if err != nil {
		return nil, fmt.Errorf("stimmen lesen: %w", err)
	}
	defer stimmen.Close()

	for stimmen.Next() {
		var rohWahl string
		var platz *int
		if err := stimmen.Scan(&rohWahl, &platz); err != nil {
			return nil, err
		}
		wahl, err := kern.WahlLesen(rohWahl)
		if err != nil {
			return nil, err
		}
		switch wahl {
		case kern.WahlJa:
			a.Ja++
		case kern.WahlNein:
			a.Nein++
		case kern.WahlEnthaltung:
			a.Enthaltung++
		}
		if platz != nil {
			a.Namentlich[*platz] = wahl
		}
	}
	return &a, stimmen.Err()
}
