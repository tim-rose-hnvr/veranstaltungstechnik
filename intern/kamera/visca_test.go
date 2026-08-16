package kamera_test

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/kameranachverfolgung/intern/kamera"
)

// TestViscaRahmen: der Rahmen für Kanal 1, Preset 5 muss Byte für Byte der
// Vorgabe entsprechen — 8 Byte Kopf, dann 81 01 04 3F 02 05 FF.
func TestViscaRahmen(t *testing.T) {
	rahmen, err := kamera.Rahmen(1, 5, 1)
	if err != nil {
		t.Fatalf("rahmen nicht gebaut: %v", err)
	}

	erwartet := []byte{
		0x01, 0x00, // Nutzlasttyp „VISCA command"
		0x00, 0x07, // Länge der Nutzlast
		0x00, 0x00, 0x00, 0x01, // Folgenummer
		0x81, 0x01, 0x04, 0x3F, 0x02, 0x05, 0xFF, // Preset abrufen
	}
	if !bytes.Equal(rahmen, erwartet) {
		t.Fatalf("rahmen falsch\nerwartet: % X\nbekommen: % X", erwartet, rahmen)
	}
}

// TestViscaRahmenKanaele prüft das Adressbyte 0x80|k und die Grenzen.
func TestViscaRahmenKanaele(t *testing.T) {
	for kanal := uint8(1); kanal <= 7; kanal++ {
		rahmen, err := kamera.Rahmen(kanal, 3, 0)
		if err != nil {
			t.Fatalf("kanal %d: %v", kanal, err)
		}
		if rahmen[8] != 0x80|kanal {
			t.Errorf("kanal %d: adressbyte %#X erwartet, %#X bekommen", kanal, 0x80|kanal, rahmen[8])
		}
	}
	if _, err := kamera.Rahmen(0, 1, 0); err == nil {
		t.Error("kanal 0 hätte abgelehnt werden müssen")
	}
	if _, err := kamera.Rahmen(8, 1, 0); err == nil {
		t.Error("kanal 8 hätte abgelehnt werden müssen")
	}
	if _, err := kamera.Rahmen(1, 200, 0); err == nil {
		t.Error("preset 200 hätte abgelehnt werden müssen")
	}
}

// TestFolgenummerZaehltHoch: je Kamera zählt die Folgenummer hoch.
func TestFolgenummerZaehltHoch(t *testing.T) {
	horcher, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testkamera nicht gestartet: %v", err)
	}
	defer horcher.Close()

	empfangen := make(chan []byte, 2)
	go func() {
		puffer := make([]byte, 64)
		for {
			n, absender, err := horcher.ReadFrom(puffer)
			if err != nil {
				return
			}
			empfangen <- append([]byte(nil), puffer[:n]...)
			// Quittung, damit der Aufruf nicht ins Zeitlimit läuft.
			horcher.WriteTo([]byte{0x01, 0x11, 0x00, 0x03, 0, 0, 0, 0, 0x90, 0x41, 0xFF}, absender)
		}
	}()

	steuerung := kamera.NeuViscaIP(500 * time.Millisecond)
	adresse := horcher.LocalAddr().String()

	for i := 1; i <= 2; i++ {
		if err := steuerung.PresetAbrufen(context.Background(), adresse, 1, uint8(i)); err != nil {
			t.Fatalf("abruf %d fehlgeschlagen: %v", i, err)
		}
		select {
		case rahmen := <-empfangen:
			if folge := rahmen[7]; folge != byte(i) {
				t.Errorf("abruf %d: folgenummer %d erwartet, %d bekommen", i, i, folge)
			}
			if preset := rahmen[13]; preset != byte(i) {
				t.Errorf("abruf %d: preset %d erwartet, %d bekommen", i, i, preset)
			}
		case <-time.After(time.Second):
			t.Fatalf("abruf %d kam nicht an", i)
		}
	}
}

// TestKameraZeitlimit: antwortet niemand, kommt ein Fehler statt eines Hängers.
func TestKameraZeitlimit(t *testing.T) {
	horcher, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testkamera nicht gestartet: %v", err)
	}
	defer horcher.Close() // liest nie, antwortet nie

	steuerung := kamera.NeuViscaIP(50 * time.Millisecond)
	beginn := time.Now()
	if err := steuerung.PresetAbrufen(context.Background(), horcher.LocalAddr().String(), 1, 1); err == nil {
		t.Fatal("ohne antwort war ein fehler erwartet")
	}
	if gedauert := time.Since(beginn); gedauert > time.Second {
		t.Errorf("zeitlimit nicht eingehalten, es dauerte %s", gedauert)
	}
}
