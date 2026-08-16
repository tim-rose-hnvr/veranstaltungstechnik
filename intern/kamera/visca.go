// Paket kamera steuert PTZ-Kameras über VISCA over IP (UDP, Standardport 52381).
package kamera

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
)

// Standardport für VISCA over IP.
const Standardport = 52381

// nutzlasttypBefehl ist der Kopfwert für „VISCA command".
var nutzlasttypBefehl = [2]byte{0x01, 0x00}

// Rahmen baut das vollständige Paket für „Preset abrufen".
//
// Nutzlast (7 Byte), Kanal k, Preset p:
//
//	0x80|k  0x01  0x04  0x3F  0x02  p  0xFF
//
// Davor der 8-Byte-Kopf:
//
//	0x01 0x00              Nutzlasttyp „VISCA command"
//	0xLL 0xLL              Länge der Nutzlast, big endian
//	0xSS 0xSS 0xSS 0xSS    Folgenummer, big endian
func Rahmen(kanal, preset uint8, folge uint32) ([]byte, error) {
	if kanal < 1 || kanal > 7 {
		return nil, fmt.Errorf("kanal %d liegt außerhalb von 1..7", kanal)
	}
	if preset > 0x7F {
		return nil, fmt.Errorf("preset %d liegt außerhalb von 0..127", preset)
	}

	nutzlast := []byte{0x80 | kanal, 0x01, 0x04, 0x3F, 0x02, preset, 0xFF}

	rahmen := make([]byte, 0, 8+len(nutzlast))
	rahmen = append(rahmen, nutzlasttypBefehl[0], nutzlasttypBefehl[1])
	rahmen = binary.BigEndian.AppendUint16(rahmen, uint16(len(nutzlast)))
	rahmen = binary.BigEndian.AppendUint32(rahmen, folge)
	return append(rahmen, nutzlast...), nil
}

// ViscaIP spricht mit den Kameras. Die Folgenummer zählt je Adresse hoch.
type ViscaIP struct {
	zeitlimit time.Duration

	mu    sync.Mutex
	folge map[string]uint32
}

// NeuViscaIP erzeugt die Steuerung. zeitlimit gilt für die Antwort der Kamera.
func NeuViscaIP(zeitlimit time.Duration) *ViscaIP {
	if zeitlimit <= 0 {
		zeitlimit = 500 * time.Millisecond
	}
	return &ViscaIP{zeitlimit: zeitlimit, folge: make(map[string]uint32)}
}

// PresetAbrufen fährt die Kamera an der Adresse auf den gespeicherten Preset.
// Antwortet sie nicht innerhalb des Zeitlimits, kommt ein Fehler zurück — für
// den Kern ist das kein Systemfehler, sondern ein Protokolleintrag.
func (v *ViscaIP) PresetAbrufen(ctx context.Context, adresse string, kanal, preset uint8) error {
	rahmen, err := Rahmen(kanal, preset, v.naechsteFolge(adresse))
	if err != nil {
		return err
	}
	if _, _, err := net.SplitHostPort(adresse); err != nil {
		adresse = net.JoinHostPort(adresse, fmt.Sprint(Standardport))
	}

	frist := time.Now().Add(v.zeitlimit)
	if ende, gesetzt := ctx.Deadline(); gesetzt && ende.Before(frist) {
		frist = ende
	}

	waehler := net.Dialer{Deadline: frist}
	verbindung, err := waehler.DialContext(ctx, "udp", adresse)
	if err != nil {
		return fmt.Errorf("kamera %s nicht erreichbar: %w", adresse, err)
	}
	defer verbindung.Close()

	if err := verbindung.SetDeadline(frist); err != nil {
		return err
	}
	if _, err := verbindung.Write(rahmen); err != nil {
		return fmt.Errorf("kamera %s: senden fehlgeschlagen: %w", adresse, err)
	}

	// VISCA over IP quittiert mit Acknowledge und Completion. Uns genügt, dass
	// überhaupt eine Antwort kommt — sonst gilt die Kamera als nicht erreichbar.
	antwort := make([]byte, 64)
	if _, err := verbindung.Read(antwort); err != nil {
		return fmt.Errorf("kamera %s: keine antwort innerhalb %s: %w", adresse, v.zeitlimit, err)
	}
	return nil
}

func (v *ViscaIP) naechsteFolge(adresse string) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.folge[adresse]++
	return v.folge[adresse]
}
