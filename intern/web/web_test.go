package web_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/siegel"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/vorabcheck"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/web"
)

// Diese Tests fahren das WebSocket-Protokoll über eine echte HTTP-Verbindung.
// Sie halten fest, was auf der Leitung steht — nicht nur, was der Kern denkt.

type stilleKamera struct{}

func (stilleKamera) PresetAbrufen(ctx context.Context, adresse string, kanal, preset uint8) error {
	return nil
}

// nachricht ist die Sicht des Clients auf den Zustand.
type nachricht struct {
	Typ     string `json:"typ"`
	Code    string `json:"code"`
	Text    string `json:"text"`
	Stand   uint64 `json:"stand"`
	Sitzung struct {
		Titel        string `json:"titel"`
		Zustand      string `json:"zustand"`
		LeitungPlatz int    `json:"leitung_platz"`
	} `json:"sitzung"`
	Ich *struct {
		Platz      int      `json:"platz"`
		Person     string   `json:"person"`
		Rolle      string   `json:"rolle"`
		Darf       []string `json:"darf"`
		Unterlagen []struct {
			ID    string `json:"id"`
			Titel string `json:"titel"`
			Stufe string `json:"stufe"`
		} `json:"unterlagen"`
	} `json:"ich"`
	// Antwort auf unterlage_abrufen.
	Marke         string `json:"marke"`
	Wasserzeichen string `json:"wasserzeichen"`
	Plaetze       []struct {
		Nummer  int    `json:"nummer"`
		Person  string `json:"person"`
		Mikro   bool   `json:"mikro"`
		Belegt  bool   `json:"belegt"`
		HatWort bool   `json:"hat_wort"`
	} `json:"plaetze"`
	Redeliste []struct {
		Platz   int    `json:"platz"`
		Person  string `json:"person"`
		Zustand string `json:"zustand"`
	} `json:"redeliste"`
}

func aufstellen(t *testing.T) *httptest.Server {
	t.Helper()
	return aufbauenMit(t, false, "")
}

// aufstellenMitMappe legt eine Unterlage auf der Platte an und hängt sie an
// den ersten Tagesordnungspunkt.
func aufstellenMitMappe(t *testing.T) *httptest.Server {
	t.Helper()
	ordner := t.TempDir()
	if err := os.WriteFile(filepath.Join(ordner, "vorlage.pdf"), []byte("Beschlussvorlage"), 0o600); err != nil {
		t.Fatalf("unterlage schreiben: %v", err)
	}
	return aufbauenMit(t, false, ordner)
}

// aufstellenMitPruefstelle schaltet zusätzlich /emulator frei.
func aufstellenMitPruefstelle(t *testing.T) *httptest.Server {
	t.Helper()
	return aufbauenMit(t, true, "")
}

func aufbauenMit(t *testing.T, pruefstelle bool, mappenordner string) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()

	saalID, plaetze, err := ablage.SaalImportieren(ctx, speicher.Saaldaten{
		Saal:    "Testraum",
		Kameras: []speicher.Kameradaten{{Name: "PTZ Mitte", Adresse: "127.0.0.1:52381", Kanal: 1}},
		Plaetze: []speicher.Platzdaten{
			{Nummer: 1, Name: "Vorsitz", Kamera: "PTZ Mitte", Preset: 1},
			{Nummer: 2, Name: "Platz 2", Kamera: "PTZ Mitte", Preset: 2},
			{Nummer: 3, Name: "Platz 3", Kamera: "PTZ Mitte", Preset: 3},
		},
	})
	if err != nil {
		t.Fatalf("saal einlesen: %v", err)
	}

	var tagesordnung []speicher.Topdaten
	if mappenordner != "" {
		tagesordnung = []speicher.Topdaten{{Nummer: 1, Titel: "Beschluss",
			Unterlagen: []speicher.Unterlagedaten{
				{Titel: "Beschlussvorlage", Datei: "vorlage.pdf", Stufe: "vertraulich"}}}}
	}

	stand, err := ablage.SitzungImportieren(ctx, saalID, speicher.Sitzungsdaten{
		Titel:        "Probesitzung",
		Tagesordnung: tagesordnung,
		Teilnahmen: []speicher.Teilnahmedaten{
			{Platz: 1, Person: "Anke Bergmann", Rolle: "leitung", Pin: "1111"},
			{Platz: 2, Person: "Jonas Öztürk", Rolle: "delegierter", Pin: "2222"},
			{Platz: 3, Person: "Rita Falk", Rolle: "schriftfuehrung", Pin: "3333"},
		},
	}.MitVerzeichnis(mappenordner))
	if err != nil {
		t.Fatalf("sitzung einlesen: %v", err)
	}

	aufbau := kern.Aufbau{
		SaalID: saalID, SitzungID: stand.SitzungID, Titel: stand.Titel,
		SitzungZustand: stand.Zustand, Plaetze: plaetze, Teilnahmen: stand.Teilnahmen,
		Tagesordnung: stand.Tagesordnung, Unterlagen: stand.Unterlagen,
		MaxOffen: 3, Zeitlimit: 50 * time.Millisecond,
	}
	k, err := kern.Neu(aufbau, stilleKamera{}, ablage, nil)
	if err != nil {
		t.Fatalf("kern nicht aufgebaut: %v", err)
	}

	oberflaeche := web.Neu(k, "../../web", nil)
	schluessel, err := siegel.Laden(filepath.Join(t.TempDir(), "kette.key"))
	if err != nil {
		t.Fatalf("siegelschlüssel: %v", err)
	}
	oberflaeche.SetzeSiegler(siegel.Neu(saalID, schluessel, ablage, nil),
		schluessel.Oeffentlich, saalID, ablage)
	oberflaeche.SetzeVorabcheck(vorabcheck.Neu(aufbau, k, stilleKamera{}, ablage, 50*time.Millisecond))
	if pruefstelle {
		oberflaeche.SetzeEmulator(web.Emulatordaten{
			Saal: "Testraum", SaalID: saalID, Plaetze: plaetze,
			Pins:    map[int]string{1: "1111", 2: "2222", 3: "3333"},
			Kameras: []web.Kameraangabe{{Name: "PTZ Mitte", Adresse: "127.0.0.1:52381", Kanal: 1}},
			Kette:   ablage,
		})
	}

	dienst := httptest.NewServer(oberflaeche.Handler())
	t.Cleanup(dienst.Close)
	return dienst
}

type client struct {
	t    *testing.T
	sitz *websocket.Conn
	ctx  context.Context
}

func verbinden(t *testing.T, dienst *httptest.Server) *client {
	t.Helper()
	ctx, ende := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(ende)

	adresse := "ws" + strings.TrimPrefix(dienst.URL, "http") + "/ws"
	sitz, _, err := websocket.Dial(ctx, adresse, nil)
	if err != nil {
		t.Fatalf("verbinden: %v", err)
	}
	t.Cleanup(func() { sitz.CloseNow() }) //nolint:errcheck

	k := &client{t: t, sitz: sitz, ctx: ctx}
	k.warte(func(n nachricht) bool { return n.Typ == "zustand" }) // Anfangszustand
	return k
}

func (k *client) schicken(inhalt map[string]any) {
	k.t.Helper()
	roh, err := json.Marshal(inhalt)
	if err != nil {
		k.t.Fatalf("nachricht verpacken: %v", err)
	}
	if err := k.sitz.Write(k.ctx, websocket.MessageText, roh); err != nil {
		k.t.Fatalf("senden: %v", err)
	}
}

// warte liest, bis die Bedingung zutrifft oder ein Fehler kommt.
func (k *client) warte(passt func(nachricht) bool) nachricht {
	k.t.Helper()
	frist, ende := context.WithTimeout(k.ctx, 3*time.Second)
	defer ende()

	for {
		_, roh, err := k.sitz.Read(frist)
		if err != nil {
			k.t.Fatalf("lesen: %v", err)
		}
		var n nachricht
		if err := json.Unmarshal(roh, &n); err != nil {
			k.t.Fatalf("nachricht nicht lesbar: %v", err)
		}
		if n.Typ == "fehler" || passt(n) {
			return n
		}
	}
}

func (k *client) anmelden(platz int, pin string) nachricht {
	k.t.Helper()
	k.schicken(map[string]any{"typ": "anmelden", "platz": platz, "pin": pin})
	return k.warte(func(n nachricht) bool { return n.Ich != nil })
}

func enthaelt(liste []string, wert string) bool {
	for _, e := range liste {
		if e == wert {
			return true
		}
	}
	return false
}

// TestProtokollAnmeldung: falsche PIN wird abgewiesen, richtige liefert den
// eigenen Zustand — und der ist je Verbindung verschieden.
func TestProtokollAnmeldung(t *testing.T) {
	dienst := aufstellen(t)
	anke := verbinden(t, dienst)
	jonas := verbinden(t, dienst)

	anke.schicken(map[string]any{"typ": "anmelden", "platz": 1, "pin": "0000"})
	if n := anke.warte(func(n nachricht) bool { return n.Typ == "fehler" }); n.Code != kern.CodePinFalsch {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodePinFalsch, n.Code)
	}

	n := anke.anmelden(1, "1111")
	if n.Ich.Person != "Anke Bergmann" || n.Ich.Rolle != string(kern.RolleLeitung) {
		t.Fatalf("falscher eigener zustand: %+v", n.Ich)
	}
	if !enthaelt(n.Ich.Darf, kern.AktionSitzungEroeffnen) {
		t.Errorf("die leitung sollte eröffnen dürfen, darf: %v", n.Ich.Darf)
	}

	// Dieselbe Änderung, andere Verbindung: "ich" ist die einzige Stelle,
	// an der nicht alle dasselbe bekommen.
	zweite := jonas.warte(func(n nachricht) bool { return n.Plaetze[0].Belegt })
	if zweite.Ich != nil {
		t.Errorf("ein nicht angemeldeter client darf kein ich bekommen: %+v", zweite.Ich)
	}
}

// TestProtokollWortUndMikro: der ganze Weg über die Leitung — melden,
// erteilen, sprechen.
func TestProtokollWortUndMikro(t *testing.T) {
	dienst := aufstellen(t)
	anke := verbinden(t, dienst)
	jonas := verbinden(t, dienst)

	anke.anmelden(1, "1111")
	jonas.anmelden(2, "2222")

	anke.schicken(map[string]any{"typ": kern.AktionSitzungEroeffnen})
	anke.warte(func(n nachricht) bool { return n.Sitzung.Zustand == string(kern.SitzungLaufend) })

	// Ohne erteiltes Wort bleibt das Mikrofon zu.
	jonas.schicken(map[string]any{"typ": "mikro_an", "platz": 2})
	if n := jonas.warte(func(n nachricht) bool { return n.Typ == "fehler" }); n.Code != kern.CodeKeinWort {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeKeinWort, n.Code)
	}

	jonas.schicken(map[string]any{"typ": kern.AktionWortMelden})
	n := jonas.warte(func(n nachricht) bool { return len(n.Redeliste) == 1 })
	if n.Redeliste[0].Person != "Jonas Öztürk" || n.Redeliste[0].Zustand != string(kern.WortGemeldet) {
		t.Fatalf("redeliste falsch: %+v", n.Redeliste[0])
	}

	anke.schicken(map[string]any{"typ": kern.AktionWortErteilen, "platz": 2})
	n = jonas.warte(func(n nachricht) bool { return n.Plaetze[1].HatWort })
	if n.Redeliste[0].Zustand != string(kern.WortErteilt) {
		t.Errorf("erteilt erwartet, %q bekommen", n.Redeliste[0].Zustand)
	}

	jonas.schicken(map[string]any{"typ": "mikro_an", "platz": 2})
	n = jonas.warte(func(n nachricht) bool { return n.Plaetze[1].Mikro })
	if n.Redeliste[0].Zustand != string(kern.WortLaufend) {
		t.Errorf("laufend erwartet, %q bekommen", n.Redeliste[0].Zustand)
	}

	// Der Entzug schließt das Mikrofon und räumt die Liste.
	anke.schicken(map[string]any{"typ": kern.AktionWortEntziehen, "platz": 2})
	n = anke.warte(func(n nachricht) bool { return len(n.Redeliste) == 0 })
	if n.Plaetze[1].Mikro {
		t.Error("das mikrofon sollte nach dem entzug zu sein")
	}
}

// TestProtokollRechteImKern: die Oberfläche filtert nichts weg. Ein Befehl
// ohne Recht geht durch bis zum Kern und wird dort abgewiesen.
func TestProtokollRechteImKern(t *testing.T) {
	dienst := aufstellen(t)
	anke := verbinden(t, dienst)
	rita := verbinden(t, dienst)
	fremder := verbinden(t, dienst)

	anke.anmelden(1, "1111")
	rita.anmelden(3, "3333")
	anke.schicken(map[string]any{"typ": kern.AktionSitzungEroeffnen})
	anke.warte(func(n nachricht) bool { return n.Sitzung.Zustand == string(kern.SitzungLaufend) })

	// Schriftführung darf nicht schalten.
	rita.schicken(map[string]any{"typ": "mikro_an", "platz": 3})
	if n := rita.warte(func(n nachricht) bool { return n.Typ == "fehler" }); n.Code != kern.CodeNichtBerechtigt {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeNichtBerechtigt, n.Code)
	}

	// Wer gar nicht angemeldet ist, darf erst recht nichts.
	fremder.schicken(map[string]any{"typ": kern.AktionWortErteilen, "platz": 2})
	if n := fremder.warte(func(n nachricht) bool { return n.Typ == "fehler" }); n.Code != kern.CodeNichtBerechtigt {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeNichtBerechtigt, n.Code)
	}
}

// TestProtokollUnbekannteNachricht: unbekannte Typen werden ignoriert, nicht
// beantwortet — und ändern nichts.
func TestProtokollUnbekannteNachricht(t *testing.T) {
	dienst := aufstellen(t)
	anke := verbinden(t, dienst)
	n := anke.anmelden(1, "1111")
	vorher := n.Stand

	anke.schicken(map[string]any{"typ": "gibt_es_nicht", "platz": 1})
	anke.schicken(map[string]any{"typ": kern.AktionSitzungEroeffnen})

	nachher := anke.warte(func(n nachricht) bool { return n.Sitzung.Zustand == string(kern.SitzungLaufend) })
	if nachher.Stand != vorher+1 {
		t.Errorf("die unbekannte nachricht hat den stand verändert: %d -> %d", vorher, nachher.Stand)
	}
}

// TestSeitenWerdenAusgeliefert: jede Rolle hat ihre Seite.
func TestSeitenWerdenAusgeliefert(t *testing.T) {
	dienst := aufstellen(t)

	for _, pfad := range []string{"/", "/namensschild?platz=1", "/dolmetscher", "/testumgebung", "/vorabcheck"} {
		antwort, err := http.Get(dienst.URL + pfad)
		if err != nil {
			t.Fatalf("%s: %v", pfad, err)
		}
		antwort.Body.Close()
		if antwort.StatusCode != http.StatusOK {
			t.Errorf("%s: HTTP %d erwartet, %d bekommen", pfad, http.StatusOK, antwort.StatusCode)
		}
		if typ := antwort.Header.Get("Content-Type"); !strings.HasPrefix(typ, "text/html") {
			t.Errorf("%s: text/html erwartet, %q bekommen", pfad, typ)
		}
	}
}

// TestVorabcheckUeberHTTP: der Bericht kommt als JSON, und während einer
// laufenden Sitzung wird abgelehnt.
func TestVorabcheckUeberHTTP(t *testing.T) {
	dienst := aufstellen(t)

	antwort, err := http.Post(dienst.URL+"/vorabcheck", "", nil)
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	defer antwort.Body.Close()
	if antwort.StatusCode != http.StatusOK {
		t.Fatalf("HTTP 200 erwartet, %d bekommen", antwort.StatusCode)
	}

	var bericht vorabcheck.Bericht
	if err := json.NewDecoder(antwort.Body).Decode(&bericht); err != nil {
		t.Fatalf("bericht nicht lesbar: %v", err)
	}
	if !bericht.Bereit || len(bericht.Punkte) == 0 {
		t.Errorf("bereit erwartet, bekommen: %+v", bericht)
	}

	// Läuft die Sitzung, ist der Check gesperrt — er bewegt Kameras.
	anke := verbinden(t, dienst)
	anke.anmelden(1, "1111")
	anke.schicken(map[string]any{"typ": kern.AktionSitzungEroeffnen})
	anke.warte(func(n nachricht) bool { return n.Sitzung.Zustand == string(kern.SitzungLaufend) })

	gesperrt, err := http.Post(dienst.URL+"/vorabcheck", "", nil)
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	defer gesperrt.Body.Close()
	if gesperrt.StatusCode != http.StatusConflict {
		t.Errorf("HTTP 409 erwartet, %d bekommen", gesperrt.StatusCode)
	}
}

// TestPruefstelleNurMitSchalter: die Prüfstelle gibt die PINs im Klartext
// heraus. Ohne ausdrückliche Freischaltung darf es ihre Adressen nicht geben —
// nicht abgeschaltet, sondern nicht vorhanden.
func TestPruefstelleNurMitSchalter(t *testing.T) {
	adressen := []string{"/emulator", "/emulator/aufbau.json", "/emulator/kette.json", "/emulator/kameras.json"}

	dienst := aufstellen(t)
	for _, pfad := range adressen {
		antwort, err := http.Get(dienst.URL + pfad)
		if err != nil {
			t.Fatalf("%s: %v", pfad, err)
		}
		antwort.Body.Close()
		if antwort.StatusCode != http.StatusNotFound {
			t.Errorf("%s: ohne freischaltung HTTP 404 erwartet, %d bekommen", pfad, antwort.StatusCode)
		}
	}

	// Mit Freischaltung sind sie da — und die PIN steht darin, wie angekündigt.
	mitSchalter := aufstellenMitPruefstelle(t)
	for _, pfad := range adressen {
		antwort, err := http.Get(mitSchalter.URL + pfad)
		if err != nil {
			t.Fatalf("%s: %v", pfad, err)
		}
		roh, _ := io.ReadAll(antwort.Body)
		antwort.Body.Close()
		if antwort.StatusCode != http.StatusOK {
			t.Errorf("%s: HTTP 200 erwartet, %d bekommen", pfad, antwort.StatusCode)
		}
		if pfad == "/emulator/aufbau.json" && !strings.Contains(string(roh), `"pin":"1111"`) {
			t.Errorf("die prüfstelle liefert die pin nicht: %s", roh)
		}
	}
}

// TestUnterlageNurGegenMarke: die Auslieferung kennt den Platz nicht, sie
// kennt nur die Marke. Ohne gültige Marke gibt es die Datei nicht.
func TestUnterlageNurGegenMarke(t *testing.T) {
	dienst := aufstellen(t)

	for _, marke := range []string{"erfunden", "0000000000000000"} {
		antwort, err := http.Get(dienst.URL + "/unterlage/" + marke)
		if err != nil {
			t.Fatalf("abruf: %v", err)
		}
		antwort.Body.Close()
		if antwort.StatusCode == http.StatusOK {
			t.Errorf("marke %q lieferte eine unterlage aus", marke)
		}
	}
}

// TestUnterlageUeberDenGanzenWeg: anmelden, Mappe im Zustand sehen, Freigabe
// holen, Datei abrufen — und die Marke ist danach verbraucht.
func TestUnterlageUeberDenGanzenWeg(t *testing.T) {
	dienst := aufstellenMitMappe(t)
	k := verbinden(t, dienst)

	angemeldet := k.anmelden(1, "1111")
	if angemeldet.Ich == nil || len(angemeldet.Ich.Unterlagen) != 1 {
		t.Fatalf("die sitzungsmappe steht nicht im zustand: %+v", angemeldet.Ich)
	}
	unterlage := angemeldet.Ich.Unterlagen[0]

	k.schicken(map[string]any{"typ": "unterlage_abrufen", "unterlage": unterlage.ID})
	freigabe := k.warte(func(n nachricht) bool { return n.Typ == "unterlage" })
	if freigabe.Marke == "" {
		t.Fatalf("keine marke bekommen: %+v", freigabe)
	}
	if !strings.Contains(freigabe.Wasserzeichen, "Anke Bergmann") {
		t.Errorf("das wasserzeichen %q nennt die person nicht", freigabe.Wasserzeichen)
	}

	antwort, err := http.Get(dienst.URL + "/unterlage/" + freigabe.Marke)
	if err != nil {
		t.Fatalf("abruf: %v", err)
	}
	roh, _ := io.ReadAll(antwort.Body)
	antwort.Body.Close()
	if antwort.StatusCode != http.StatusOK {
		t.Fatalf("HTTP 200 erwartet, %d bekommen: %s", antwort.StatusCode, roh)
	}
	if string(roh) != "Beschlussvorlage" {
		t.Errorf("inhalt %q bekommen", roh)
	}
	if kopf := antwort.Header.Get("X-Wasserzeichen"); kopf != freigabe.Wasserzeichen {
		t.Errorf("das wasserzeichen fehlt im kopf: %q", kopf)
	}
	if kopf := antwort.Header.Get("Cache-Control"); !strings.Contains(kopf, "no-store") {
		t.Errorf("eine vertrauliche unterlage darf nicht zwischengespeichert werden: %q", kopf)
	}

	// Zweiter Abruf mit derselben Marke: nichts mehr.
	zweite, err := http.Get(dienst.URL + "/unterlage/" + freigabe.Marke)
	if err != nil {
		t.Fatalf("zweiter abruf: %v", err)
	}
	zweite.Body.Close()
	if zweite.StatusCode == http.StatusOK {
		t.Error("die marke ließ sich ein zweites mal einlösen")
	}
}

// TestSiegelUeberHTTP: die Kette lässt sich abschließen und die Prüfung sagt
// danach, wie viel gedeckt ist. Ohne Prüfstelle, denn das gehört in jeden Saal.
func TestSiegelUeberHTTP(t *testing.T) {
	dienst := aufstellen(t)

	// Erst etwas in die Kette bringen.
	k := verbinden(t, dienst)
	k.anmelden(1, "1111")

	vorher := siegelbericht(t, dienst)
	if vorher["ok"] != true {
		t.Fatalf("eine ungesiegelte kette ist in ordnung: %+v", vorher)
	}
	if vorher["siegel"].(float64) != 0 {
		t.Errorf("noch kein siegel erwartet: %+v", vorher)
	}

	antwort, err := http.Post(dienst.URL+"/siegel", "", nil)
	if err != nil {
		t.Fatalf("siegeln: %v", err)
	}
	var abschluss map[string]any
	json.NewDecoder(antwort.Body).Decode(&abschluss) //nolint:errcheck
	antwort.Body.Close()
	if antwort.StatusCode != http.StatusOK || abschluss["neu"] != true {
		t.Fatalf("HTTP %d, abschluss %+v", antwort.StatusCode, abschluss)
	}

	nachher := siegelbericht(t, dienst)
	if nachher["ok"] != true {
		t.Fatalf("die prüfung schlägt fehl: %+v", nachher)
	}
	if nachher["siegel"].(float64) != 1 {
		t.Errorf("ein siegel erwartet: %+v", nachher)
	}
	if nachher["gedeckt"].(float64) < 1 {
		t.Errorf("das siegel deckt nichts: %+v", nachher)
	}
	if abdruecke, passt := nachher["fingerabdruecke"].([]any); !passt || len(abdruecke) != 1 {
		t.Errorf("genau ein fingerabdruck erwartet: %+v", nachher["fingerabdruecke"])
	}
}

func siegelbericht(t *testing.T, dienst *httptest.Server) map[string]any {
	t.Helper()
	antwort, err := http.Get(dienst.URL + "/siegel.json")
	if err != nil {
		t.Fatalf("siegel.json: %v", err)
	}
	defer antwort.Body.Close()
	if antwort.StatusCode != http.StatusOK {
		t.Fatalf("HTTP %d von siegel.json", antwort.StatusCode)
	}
	var bericht map[string]any
	if err := json.NewDecoder(antwort.Body).Decode(&bericht); err != nil {
		t.Fatalf("bericht nicht lesbar: %v", err)
	}
	return bericht
}
