// Paket kern enthält den Sitzungszustand, die Regeln und die Ereigniskette.
// Es kennt weder Datenbank noch HTTP noch UDP — beides wird über Schnittstellen
// hereingereicht.
package kern

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"
)

// Ereignis ist ein Eintrag der fortlaufenden, nur anfügbaren Hash-Kette eines
// Saals. Es wird nie geändert und nie gelöscht.
type Ereignis struct {
	FolgeNr        int64
	Zeit           time.Time
	Art            string
	Nutzlast       map[string]any
	VorgaengerHash []byte
	Hash           []byte
}

// ZeitText ist die kanonische Textform der Zeit im Hash: RFC3339 mit
// Nanosekunden, immer in UTC.
func ZeitText(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// ZeitStutzen kürzt auf Mikrosekunden. PostgreSQL speichert timestamptz mit
// Mikrosekunden-Auflösung; ohne diesen Schnitt wäre der gehashte Zeitwert ein
// anderer als der gespeicherte und die Kette ließe sich nach dem Neuladen
// nicht mehr prüfen.
func ZeitStutzen(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}

// Kanonisch erzeugt die kanonische JSON-Form der Nutzlast. encoding/json
// sortiert die Schlüssel einer Map und setzt keine Leerzeichen — damit ist die
// Form eindeutig und nach dem Umweg über jsonb reproduzierbar.
func Kanonisch(nutzlast map[string]any) ([]byte, error) {
	if nutzlast == nil {
		nutzlast = map[string]any{}
	}
	return json.Marshal(nutzlast)
}

// HashBerechnen bildet
// sha256(vorgaenger_hash || folge_nr || zeit_rfc3339nano || art || nutzlast_kanonisches_json).
// folge_nr geht als 8 Byte big endian ein, alles Übrige als UTF-8-Bytes.
// Beim ersten Ereignis ist vorgaenger leer.
func HashBerechnen(vorgaenger []byte, folgeNr int64, zeit time.Time, art string, nutzlast []byte) []byte {
	h := sha256.New()
	h.Write(vorgaenger)
	var nr [8]byte
	binary.BigEndian.PutUint64(nr[:], uint64(folgeNr))
	h.Write(nr[:])
	h.Write([]byte(ZeitText(zeit)))
	h.Write([]byte(art))
	h.Write(nutzlast)
	return h.Sum(nil)
}

// NaechstesEreignis baut den nächsten Kettenglied auf dem Vorgänger auf.
// vorgaenger ist nil, wenn der Saal noch kein Ereignis hat.
func NaechstesEreignis(vorgaenger *Ereignis, zeit time.Time, art string, nutzlast map[string]any) (Ereignis, error) {
	if art == "" {
		return Ereignis{}, fmt.Errorf("ereignis ohne art")
	}
	roh, err := Kanonisch(nutzlast)
	if err != nil {
		return Ereignis{}, fmt.Errorf("nutzlast nicht kanonisierbar: %w", err)
	}

	e := Ereignis{
		FolgeNr:  1,
		Zeit:     ZeitStutzen(zeit),
		Art:      art,
		Nutzlast: nutzlast,
	}
	if vorgaenger != nil {
		e.FolgeNr = vorgaenger.FolgeNr + 1
		e.VorgaengerHash = vorgaenger.Hash
	}
	e.Hash = HashBerechnen(e.VorgaengerHash, e.FolgeNr, e.Zeit, e.Art, roh)
	return e, nil
}

// KettePruefen prüft eine vollständige Kette: beginnt bei 1, keine Lücke in
// folge_nr, jeder Vorgängerhash passt, jeder Hash ist nachrechenbar.
func KettePruefen(kette []Ereignis) error {
	var vorher *Ereignis
	for i := range kette {
		e := kette[i]

		erwartet := int64(1)
		if vorher != nil {
			erwartet = vorher.FolgeNr + 1
		}
		if e.FolgeNr != erwartet {
			return fmt.Errorf("lücke in der kette: folge_nr %d erwartet, %d gefunden", erwartet, e.FolgeNr)
		}

		var vorgaengerHash []byte
		if vorher != nil {
			vorgaengerHash = vorher.Hash
		}
		if !bytes.Equal(e.VorgaengerHash, vorgaengerHash) {
			return fmt.Errorf("folge_nr %d: vorgaenger_hash passt nicht", e.FolgeNr)
		}

		roh, err := Kanonisch(e.Nutzlast)
		if err != nil {
			return fmt.Errorf("folge_nr %d: nutzlast nicht kanonisierbar: %w", e.FolgeNr, err)
		}
		if !bytes.Equal(e.Hash, HashBerechnen(e.VorgaengerHash, e.FolgeNr, e.Zeit, e.Art, roh)) {
			return fmt.Errorf("folge_nr %d: hash stimmt nicht", e.FolgeNr)
		}

		vorher = &kette[i]
	}
	return nil
}
