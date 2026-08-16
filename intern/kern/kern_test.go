package kern_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/kameranachverfolgung/intern/kern"
)

// stilleKamera nimmt jeden Befehl an, ohne UDP zu sprechen.
type stilleKamera struct {
	mu      sync.Mutex
	abrufe  []abruf
	antwort error
}

type abruf struct {
	Adresse string
	Kanal   uint8
	Preset  uint8
}

func (s *stilleKamera) PresetAbrufen(ctx context.Context, adresse string, kanal, preset uint8) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.abrufe = append(s.abrufe, abruf{adresse, kanal, preset})
	return s.antwort
}

func (s *stilleKamera) Abrufe() []abruf {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]abruf(nil), s.abrufe...)
}

// zaehlablage merkt sich nur, welche Ereignisse geschrieben wurden.
type zaehlablage struct {
	mu    sync.Mutex
	arten []string
}

func (z *zaehlablage) EreignisAnfuegen(ctx context.Context, saalID, art string, nutzlast map[string]any) (kern.Ereignis, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.arten = append(z.arten, art)
	return kern.Ereignis{FolgeNr: int64(len(z.arten)), Art: art}, nil
}

func (z *zaehlablage) Arten() []string {
	z.mu.Lock()
	defer z.mu.Unlock()
	return append([]string(nil), z.arten...)
}

func aufbauen(anzahl int) []kern.Platzaufbau {
	var aufbau []kern.Platzaufbau
	for i := 1; i <= anzahl; i++ {
		aufbau = append(aufbau, kern.Platzaufbau{
			Nummer:        i,
			Name:          "Platz",
			KameraName:    "PTZ Mitte",
			KameraAdresse: "192.168.1.50:52381",
			Kanal:         1,
			Preset:        uint8(i),
		})
	}
	return aufbau
}

// TestGrenzeOffenerMikrofone: bei max_offene_mikrofone 3 wird das vierte
// Mikrofon abgelehnt und der Zustand bleibt unverändert.
func TestGrenzeOffenerMikrofone(t *testing.T) {
	kamera := &stilleKamera{}
	ablage := &zaehlablage{}
	k, err := kern.Neu("saal-1", aufbauen(5), 3, 100*time.Millisecond, kamera, ablage, nil)
	if err != nil {
		t.Fatalf("kern nicht aufgebaut: %v", err)
	}
	ctx := context.Background()

	for platz := 1; platz <= 3; platz++ {
		if err := k.MikroAn(ctx, platz); err != nil {
			t.Fatalf("mikro %d ließ sich nicht öffnen: %v", platz, err)
		}
	}
	vorher := k.Zustand()

	err = k.MikroAn(ctx, 4)
	if err == nil {
		t.Fatal("das vierte mikrofon wurde angenommen, erwartet war eine ablehnung")
	}
	var f *kern.Fehler
	if !errors.As(err, &f) || f.Code != kern.CodeGrenzeErreicht {
		t.Fatalf("code %q erwartet, %v bekommen", kern.CodeGrenzeErreicht, err)
	}

	nachher := k.Zustand()
	if nachher.Stand != vorher.Stand {
		t.Errorf("stand hat sich geändert: %d -> %d", vorher.Stand, nachher.Stand)
	}
	for i, p := range nachher.Plaetze {
		if p.Mikro != vorher.Plaetze[i].Mikro {
			t.Errorf("platz %d hat sich geändert", p.Nummer)
		}
	}

	k.KameraAbwarten()
	if abrufe := kamera.Abrufe(); len(abrufe) != 3 {
		t.Errorf("3 kameraabrufe erwartet, %d bekommen", len(abrufe))
	}

	// Nach dem Schließen eines Mikrofons ist wieder Platz.
	if err := k.MikroAus(ctx, 1); err != nil {
		t.Fatalf("mikro 1 ließ sich nicht schließen: %v", err)
	}
	if err := k.MikroAn(ctx, 4); err != nil {
		t.Fatalf("mikro 4 nach dem freiwerden abgelehnt: %v", err)
	}
	k.KameraAbwarten()
}

// TestPlatzUnbekannt: ein unbekannter Platz wird abgewiesen, nicht angelegt.
func TestPlatzUnbekannt(t *testing.T) {
	k, err := kern.Neu("saal-1", aufbauen(2), 3, 100*time.Millisecond, &stilleKamera{}, &zaehlablage{}, nil)
	if err != nil {
		t.Fatalf("kern nicht aufgebaut: %v", err)
	}

	var f *kern.Fehler
	if err := k.MikroAn(context.Background(), 99); !errors.As(err, &f) || f.Code != kern.CodePlatzUnbekannt {
		t.Fatalf("code %q erwartet, %v bekommen", kern.CodePlatzUnbekannt, err)
	}
	if anzahl := len(k.Zustand().Plaetze); anzahl != 2 {
		t.Errorf("2 plätze erwartet, %d bekommen", anzahl)
	}
}

// TestPlatzBelegt: ein belegter Platz lässt sich kein zweites Mal anmelden.
func TestPlatzBelegt(t *testing.T) {
	k, err := kern.Neu("saal-1", aufbauen(2), 3, 100*time.Millisecond, &stilleKamera{}, &zaehlablage{}, nil)
	if err != nil {
		t.Fatalf("kern nicht aufgebaut: %v", err)
	}
	ctx := context.Background()

	if err := k.Anmelden(ctx, 1); err != nil {
		t.Fatalf("anmelden fehlgeschlagen: %v", err)
	}
	var f *kern.Fehler
	if err := k.Anmelden(ctx, 1); !errors.As(err, &f) || f.Code != kern.CodePlatzBelegt {
		t.Fatalf("code %q erwartet, %v bekommen", kern.CodePlatzBelegt, err)
	}
}

// TestKameraausfallLaeuftWeiter: eine nicht erreichbare Kamera hält das
// System nicht auf, sie wird protokolliert.
func TestKameraausfallLaeuftWeiter(t *testing.T) {
	kamera := &stilleKamera{antwort: errors.New("keine antwort")}
	ablage := &zaehlablage{}
	k, err := kern.Neu("saal-1", aufbauen(3), 3, 50*time.Millisecond, kamera, ablage, nil)
	if err != nil {
		t.Fatalf("kern nicht aufgebaut: %v", err)
	}

	if err := k.MikroAn(context.Background(), 2); err != nil {
		t.Fatalf("mikro trotz kameraausfall abgelehnt: %v", err)
	}
	k.KameraAbwarten()

	z := k.Zustand()
	if !z.Plaetze[1].Mikro {
		t.Error("das mikrofon sollte offen sein")
	}
	if z.Kamera == nil || z.Kamera.Erreichbar {
		t.Errorf("kamera sollte als nicht erreichbar gemeldet sein, ist %+v", z.Kamera)
	}

	var gefunden bool
	for _, art := range ablage.Arten() {
		if art == "kamera_nicht_erreichbar" {
			gefunden = true
		}
	}
	if !gefunden {
		t.Errorf("ereignis kamera_nicht_erreichbar fehlt, geschrieben wurde %v", ablage.Arten())
	}
}
