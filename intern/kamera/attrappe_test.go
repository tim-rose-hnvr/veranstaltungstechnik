package kamera_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kamera"
)

// TestAttrappeBekommtDenEchtenRahmen fährt den ganzen Weg: die Steuerung baut
// den Rahmen, schickt ihn über einen echten UDP-Sockel, die Attrappe liest ihn
// und antwortet. Wäre der Rahmen falsch, käme keine Antwort und
// PresetAbrufen schlüge fehl.
func TestAttrappeBekommtDenEchtenRahmen(t *testing.T) {
	attrappe := kamera.NeuAttrappe(nil)
	defer attrappe.Schliessen()

	adresse, err := attrappe.Anschalten("PTZ links", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("attrappe anschalten: %v", err)
	}

	steuerung := kamera.NeuViscaIP(2 * time.Second)
	if err := steuerung.PresetAbrufen(context.Background(), adresse, 1, 7); err != nil {
		t.Fatalf("preset abrufen: %v", err)
	}

	empfang := attrappe.Empfangen()
	if len(empfang) != 1 {
		t.Fatalf("ein befehl erwartet, %d angekommen", len(empfang))
	}
	e := empfang[0]
	if e.Kanal != 1 || e.Preset != 7 {
		t.Errorf("kanal 1, preset 7 erwartet: kanal %d, preset %d", e.Kanal, e.Preset)
	}
	if e.Folge != 1 {
		t.Errorf("folge 1 erwartet, %d bekommen", e.Folge)
	}
	// Der Rahmen ist der aus dem Protokoll, nicht irgendeiner.
	if erwartet := "01 00 00 07 00 00 00 01 81 01 04 3f 02 07 ff"; e.Roh != erwartet {
		t.Errorf("rahmen\n  %s\nerwartet\n  %s", e.Roh, erwartet)
	}
	if !strings.Contains(e.Deutung, "Preset 7") {
		t.Errorf("deutung %q nennt den preset nicht", e.Deutung)
	}
}

// TestAttrappeMerktSichDenStand: die Kamera weiß, wo sie steht.
func TestAttrappeMerktSichDenStand(t *testing.T) {
	attrappe := kamera.NeuAttrappe(nil)
	defer attrappe.Schliessen()

	adresse, err := attrappe.Anschalten("PTZ rechts", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("attrappe anschalten: %v", err)
	}
	steuerung := kamera.NeuViscaIP(2 * time.Second)
	for _, preset := range []uint8{3, 9} {
		if err := steuerung.PresetAbrufen(context.Background(), adresse, 1, preset); err != nil {
			t.Fatalf("preset %d: %v", preset, err)
		}
	}

	staende := attrappe.Staende()
	if len(staende) != 1 {
		t.Fatalf("eine kamera erwartet, %d bekommen", len(staende))
	}
	if staende[0].Preset != 9 || staende[0].Befehle != 2 {
		t.Errorf("preset 9 nach 2 befehlen erwartet: %+v", staende[0])
	}
}

// TestRahmenLesenWeistMuellAb: was keine gültige Anweisung ist, wird nicht als
// eine gedeutet. Sonst zeigte die Vorführung Bewegungen, die es nicht gab.
func TestRahmenLesenWeistMuellAb(t *testing.T) {
	gueltig, err := kamera.Rahmen(2, 5, 42)
	if err != nil {
		t.Fatalf("rahmen bauen: %v", err)
	}

	faelle := map[string][]byte{
		"zu kurz":            gueltig[:6],
		"falscher typ":       append([]byte{0x02, 0x00}, gueltig[2:]...),
		"länge passt nicht":  append([]byte{0x01, 0x00, 0x00, 0x09}, gueltig[4:]...),
		"kein preset-befehl": {0x01, 0x00, 0x00, 0x07, 0, 0, 0, 1, 0x81, 0x01, 0x04, 0x07, 0x02, 0x05, 0xFF},
		"endet nicht auf ff": {0x01, 0x00, 0x00, 0x07, 0, 0, 0, 1, 0x81, 0x01, 0x04, 0x3F, 0x02, 0x05, 0x00},
	}
	for name, roh := range faelle {
		if _, _, _, err := kamera.RahmenLesen(roh); err == nil {
			t.Errorf("%s: wurde als gültiger befehl gelesen", name)
		}
	}

	kanal, preset, folge, err := kamera.RahmenLesen(gueltig)
	if err != nil {
		t.Fatalf("gültiger rahmen abgewiesen: %v", err)
	}
	if kanal != 2 || preset != 5 || folge != 42 {
		t.Errorf("kanal 2, preset 5, folge 42 erwartet: %d, %d, %d", kanal, preset, folge)
	}
}
