// Paket siegel schließt die Ereigniskette ab und unterschreibt den Abschluss.
//
// Die Kette allein zeigt, dass niemand einen Eintrag geändert hat. Sie zeigt
// nicht, dass niemand die ganze Kette neu gerechnet hat: wer Schreibrechte auf
// die Datenbank hat, kann eine zweite, in sich stimmige Kette bauen.
//
// Ein Siegel schließt diese Lücke. Es unterschreibt Kopf und Länge der Kette zu
// einem Zeitpunkt mit einem Schlüssel, der nicht in der Datenbank liegt. Eine
// nachgebaute Kette hat entweder kein Siegel oder eines, das nicht aufgeht.
//
// Das Siegel liegt in der Kette selbst, nicht in einer eigenen Tabelle: eine
// zweite Liste daneben könnte auseinanderlaufen, und nur die Kette ist
// fälschungssicher.
package siegel

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
)

// Art ist die Ereignisart eines Siegels.
const Art = "kette_gesiegelt"

// Schluessel ist das Schlüsselpaar, mit dem gesiegelt wird.
type Schluessel struct {
	privat      ed25519.PrivateKey
	Oeffentlich ed25519.PublicKey
}

// Laden liest den Schlüssel von der Platte und legt ihn beim ersten Mal an.
// Die Datei bekommt 0600 — liegt sie offener, wird abgebrochen: ein Siegel,
// dessen Schlüssel jeder lesen kann, beweist nichts.
func Laden(pfad string) (*Schluessel, error) {
	roh, err := os.ReadFile(pfad)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return anlegen(pfad)
	case err != nil:
		return nil, fmt.Errorf("siegelschlüssel %s: %w", pfad, err)
	}

	lage, err := os.Stat(pfad)
	if err != nil {
		return nil, err
	}
	if lage.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("siegelschlüssel %s ist mit %o zu offen, erwartet 0600",
			pfad, lage.Mode().Perm())
	}

	saat, err := hex.DecodeString(string(bytes.TrimSpace(roh)))
	if err != nil {
		return nil, fmt.Errorf("siegelschlüssel %s nicht lesbar: %w", pfad, err)
	}
	if len(saat) != ed25519.SeedSize {
		return nil, fmt.Errorf("siegelschlüssel %s ist %d byte lang, erwartet %d",
			pfad, len(saat), ed25519.SeedSize)
	}
	privat := ed25519.NewKeyFromSeed(saat)
	return &Schluessel{privat: privat, Oeffentlich: privat.Public().(ed25519.PublicKey)}, nil
}

func anlegen(pfad string) (*Schluessel, error) {
	oeffentlich, privat, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("siegelschlüssel erzeugen: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pfad), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(pfad, []byte(hex.EncodeToString(privat.Seed())+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("siegelschlüssel %s schreiben: %w", pfad, err)
	}
	// Der öffentliche Teil kommt daneben. Er darf weitergegeben werden und ist
	// das, womit jemand von außen prüft.
	if err := os.WriteFile(pfad+".pub", []byte(hex.EncodeToString(oeffentlich)+"\n"), 0o644); err != nil {
		return nil, fmt.Errorf("öffentlichen schlüssel schreiben: %w", err)
	}
	return &Schluessel{privat: privat, Oeffentlich: oeffentlich}, nil
}

// Fingerabdruck ist die kurze Form des öffentlichen Schlüssels, wie man sie
// mündlich vergleicht.
func Fingerabdruck(oeffentlich ed25519.PublicKey) string {
	summe := sha256.Sum256(oeffentlich)
	return hex.EncodeToString(summe[:8])
}

// Aussage ist genau das, was unterschrieben wird. Sie wird beim Prüfen aus der
// Nutzlast neu gebaut — deshalb muss sie eindeutig und reproduzierbar sein.
func Aussage(saalID string, von, bis int64, kopfHash string, zeit string) ([]byte, error) {
	return kern.Kanonisch(map[string]any{
		"saal": saalID, "von": von, "bis": bis, "kopf": kopfHash, "zeit": zeit,
	})
}

// Ereignisablage ist der Ausschnitt der Ablage, den das Siegeln braucht.
type Ereignisablage interface {
	Ereignisse(ctx context.Context, saalID string) ([]kern.Ereignis, error)
	EreignisAnfuegen(ctx context.Context, saalID, art string, nutzlast map[string]any) (kern.Ereignis, error)
}

// Siegler schließt die Kette eines Saals ab.
type Siegler struct {
	saalID     string
	schluessel *Schluessel
	ablage     Ereignisablage
	protokoll  *slog.Logger
}

// Neu baut den Siegler.
func Neu(saalID string, schluessel *Schluessel, ablage Ereignisablage, protokoll *slog.Logger) *Siegler {
	if protokoll == nil {
		protokoll = slog.Default()
	}
	return &Siegler{saalID: saalID, schluessel: schluessel, ablage: ablage, protokoll: protokoll}
}

// Abschluss beschreibt ein frisch gesetztes Siegel.
type Abschluss struct {
	Von        int64  `json:"von"`
	Bis        int64  `json:"bis"`
	Kopf       string `json:"kopf"`
	Zeit       string `json:"zeit"`
	Neu        bool   `json:"neu"` // false: seit dem letzten Siegel kam nichts dazu
	Schluessel string `json:"schluessel"`
}

// Siegeln schließt die Kette bis zum jetzigen Kopf ab.
//
// Sind seit dem letzten Siegel nur Siegel dazugekommen, wird keines gesetzt:
// ein Siegel, das nur ein Siegel deckt, bezeugt nichts über den Saal und ließe
// die Kette bei jedem Aufruf um einen Eintrag wachsen.
func (s *Siegler) Siegeln(ctx context.Context) (Abschluss, error) {
	kette, err := s.ablage.Ereignisse(ctx, s.saalID)
	if err != nil {
		return Abschluss{}, fmt.Errorf("kette lesen: %w", err)
	}
	if len(kette) == 0 {
		return Abschluss{}, nil
	}
	kopf := kette[len(kette)-1]

	von := int64(1)
	if letztes, gefunden := letztesSiegel(kette); gefunden {
		von = letztes + 1
	}
	inhalt := 0
	for _, e := range kette {
		if e.FolgeNr >= von && e.Art != Art {
			inhalt++
		}
	}
	if inhalt == 0 {
		return Abschluss{Von: von, Bis: kopf.FolgeNr, Neu: false}, nil
	}

	zeit := kern.ZeitText(kern.ZeitStutzen(time.Now()))
	kopfHash := hex.EncodeToString(kopf.Hash)
	aussage, err := Aussage(s.saalID, von, kopf.FolgeNr, kopfHash, zeit)
	if err != nil {
		return Abschluss{}, err
	}
	unterschrift := ed25519.Sign(s.schluessel.privat, aussage)

	nutzlast := map[string]any{
		"von": von, "bis": kopf.FolgeNr, "kopf": kopfHash, "zeit": zeit,
		"signatur":   hex.EncodeToString(unterschrift),
		"schluessel": hex.EncodeToString(s.schluessel.Oeffentlich),
	}
	if _, err := s.ablage.EreignisAnfuegen(ctx, s.saalID, Art, nutzlast); err != nil {
		return Abschluss{}, fmt.Errorf("siegel anfügen: %w", err)
	}

	s.protokoll.Info("kette gesiegelt",
		"von", von, "bis", kopf.FolgeNr, "schluessel", Fingerabdruck(s.schluessel.Oeffentlich))
	return Abschluss{
		Von: von, Bis: kopf.FolgeNr, Kopf: kopfHash, Zeit: zeit, Neu: true,
		Schluessel: Fingerabdruck(s.schluessel.Oeffentlich),
	}, nil
}

// Taeglich siegelt jeden Tag zur angegebenen Ortszeit ("23:55") und ein
// letztes Mal, wenn der Kontext endet — ein Server, der ohne Siegel
// heruntergefahren wird, hinterlässt eine ungeschlossene Kette.
func (s *Siegler) Taeglich(ctx context.Context, uhrzeit string) error {
	stunde, minute, err := zeitLesen(uhrzeit)
	if err != nil {
		return err
	}

	go func() {
		for {
			jetzt := time.Now()
			naechste := time.Date(jetzt.Year(), jetzt.Month(), jetzt.Day(), stunde, minute, 0, 0, jetzt.Location())
			if !naechste.After(jetzt) {
				naechste = naechste.AddDate(0, 0, 1)
			}
			uhr := time.NewTimer(time.Until(naechste))
			select {
			case <-ctx.Done():
				uhr.Stop()
				// Beim Herunterfahren ohne Cancel siegeln: der Kontext ist
				// schon beendet, die Ablage steht aber noch.
				abschluss, abbrechen := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				if _, err := s.Siegeln(abschluss); err != nil {
					s.protokoll.Error("abschlusssiegel nicht gesetzt", "grund", err)
				}
				abbrechen()
				return
			case <-uhr.C:
				if _, err := s.Siegeln(ctx); err != nil {
					s.protokoll.Error("tagessiegel nicht gesetzt", "grund", err)
				}
			}
		}
	}()
	return nil
}

func zeitLesen(uhrzeit string) (int, int, error) {
	zeit, err := time.Parse("15:04", uhrzeit)
	if err != nil {
		return 0, 0, fmt.Errorf("siegel_uhrzeit %q: erwartet HH:MM", uhrzeit)
	}
	return zeit.Hour(), zeit.Minute(), nil
}

// Bericht ist das Ergebnis einer Prüfung.
type Bericht struct {
	Siegel          int      `json:"siegel"`  // wie viele geprüft wurden
	Gedeckt         int64    `json:"gedeckt"` // bis zu welcher folge_nr ein Siegel reicht
	Laenge          int64    `json:"laenge"`  // wie lang die Kette ist
	Fingerabdruecke []string `json:"fingerabdruecke"`
	Fehler          []string `json:"fehler"`
}

// Ok sagt, ob jedes Siegel aufgeht.
func (b Bericht) Ok() bool { return len(b.Fehler) == 0 }

// Ungedeckt ist die Zahl der Einträge nach dem letzten Siegel. Sie ist kein
// Fehler — laufende Sitzungen sind immer ungedeckt —, aber sie gehört gesagt.
func (b Bericht) Ungedeckt() int64 { return b.Laenge - b.Gedeckt }

// Pruefen rechnet jedes Siegel in der Kette nach. erwartet ist der öffentliche
// Schlüssel, dem geglaubt wird; ist er nil, wird gegen den Schlüssel geprüft,
// der im Siegel steht — dann beweist die Prüfung nur, dass das Siegel in sich
// stimmt, und der Fingerabdruck muss von Hand verglichen werden.
func Pruefen(saalID string, kette []kern.Ereignis, erwartet ed25519.PublicKey) Bericht {
	b := Bericht{}
	if len(kette) > 0 {
		b.Laenge = kette[len(kette)-1].FolgeNr
	}
	nach := make(map[int64]kern.Ereignis, len(kette))
	for _, e := range kette {
		nach[e.FolgeNr] = e
	}
	gesehen := map[string]bool{}

	for _, e := range kette {
		if e.Art != Art {
			continue
		}
		b.Siegel++

		von, bis := zahl(e.Nutzlast["von"]), zahl(e.Nutzlast["bis"])
		kopf := text(e.Nutzlast["kopf"])
		zeit := text(e.Nutzlast["zeit"])

		rohSchluessel, err := hex.DecodeString(text(e.Nutzlast["schluessel"]))
		if err != nil || len(rohSchluessel) != ed25519.PublicKeySize {
			b.Fehler = append(b.Fehler, fmt.Sprintf("siegel %d: schlüssel unlesbar", e.FolgeNr))
			continue
		}
		schluessel := ed25519.PublicKey(rohSchluessel)
		if erwartet != nil && !schluessel.Equal(erwartet) {
			b.Fehler = append(b.Fehler, fmt.Sprintf(
				"siegel %d: mit einem fremden schlüssel gesetzt (%s)", e.FolgeNr, Fingerabdruck(schluessel)))
			continue
		}
		if abdruck := Fingerabdruck(schluessel); !gesehen[abdruck] {
			gesehen[abdruck] = true
			b.Fingerabdruecke = append(b.Fingerabdruecke, abdruck)
		}

		unterschrift, err := hex.DecodeString(text(e.Nutzlast["signatur"]))
		if err != nil {
			b.Fehler = append(b.Fehler, fmt.Sprintf("siegel %d: signatur unlesbar", e.FolgeNr))
			continue
		}
		aussage, err := Aussage(saalID, von, bis, kopf, zeit)
		if err != nil {
			b.Fehler = append(b.Fehler, fmt.Sprintf("siegel %d: aussage nicht bildbar", e.FolgeNr))
			continue
		}
		if !ed25519.Verify(schluessel, aussage, unterschrift) {
			b.Fehler = append(b.Fehler, fmt.Sprintf("siegel %d: unterschrift geht nicht auf", e.FolgeNr))
			continue
		}

		// Die Unterschrift stimmt — deckt sie auch die Kette, die hier liegt?
		gesiegelt, gefunden := nach[bis]
		if !gefunden {
			b.Fehler = append(b.Fehler, fmt.Sprintf(
				"siegel %d: unterschreibt folge_nr %d, die es nicht gibt", e.FolgeNr, bis))
			continue
		}
		if hex.EncodeToString(gesiegelt.Hash) != kopf {
			b.Fehler = append(b.Fehler, fmt.Sprintf(
				"siegel %d: der gesiegelte kopf passt nicht zu folge_nr %d — die kette wurde ausgetauscht",
				e.FolgeNr, bis))
			continue
		}
		if bis > b.Gedeckt {
			b.Gedeckt = bis
		}
	}
	return b
}

// letztesSiegel liefert die höchste bereits gesiegelte folge_nr.
func letztesSiegel(kette []kern.Ereignis) (int64, bool) {
	var hoechste int64
	gefunden := false
	for _, e := range kette {
		if e.Art != Art {
			continue
		}
		if bis := zahl(e.Nutzlast["bis"]); bis > hoechste {
			hoechste, gefunden = bis, true
		}
	}
	return hoechste, gefunden
}

func text(wert any) string {
	if s, passt := wert.(string); passt {
		return s
	}
	return ""
}

// zahl liest eine Zahl unabhängig davon, ob sie direkt aus dem Siegler kommt
// (int64) oder den Umweg über jsonb genommen hat (float64).
func zahl(wert any) int64 {
	switch z := wert.(type) {
	case int64:
		return z
	case int:
		return int64(z)
	case float64:
		return int64(z)
	default:
		return 0
	}
}
