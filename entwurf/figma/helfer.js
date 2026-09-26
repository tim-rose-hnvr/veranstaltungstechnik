// ConferenceCue — gemeinsame Helfer für alle use_figma-Bauskripte.
// Diesen Block unverändert an den Anfang jedes Skripts setzen. Er sucht
// Variablen, Textstile und Effektstile über ihren NAMEN, nicht über Kennungen,
// damit neue Tokens ohne Nacharbeit verfügbar sind.
//
// Zwei Fallen, die dieser Block abfängt:
// 1. Farbe an eine Variable binden: den aufgelösten Wert als color mitgeben.
//    Mit color 0,0,0 als Platzhalter zeigt der Renderer sonst Schwarz, wenn
//    dieselbe Variable schon gebunden war.
// 2. resize() auf Text setzt textAutoResize zurück — text() erledigt die
//    Reihenfolge.
// 3. Effektstile heißen CC.effekte['1'|'2'|'3'|'Papier'] (Ebene/…); Rahmen mit
//    Schatten brauchen clipsContent = true und Luft im Elternrahmen.
const CC = await (async () => {
  const alle = await figma.variables.getLocalVariablesAsync();
  const vars = {}; for (const v of alle) vars[v.name] = v;
  const sammlungen = await figma.variables.getLocalVariableCollectionsAsync();
  const farbSammlung = sammlungen.find(c => c.name === 'Farbe');
  const HELL = farbSammlung.modes.find(m => m.name === 'Hell').modeId;
  const DUNKEL = farbSammlung.modes.find(m => m.name === 'Dunkel').modeId;
  const cache = {};
  async function wert(v, modus) {
    const key = v.id + modus; if (cache[key]) return cache[key];
    let val = v.valuesByMode[modus] ?? Object.values(v.valuesByMode)[0];
    while (val && val.type === 'VARIABLE_ALIAS') {
      const a = await figma.variables.getVariableByIdAsync(val.id);
      val = a.valuesByMode[modus] ?? Object.values(a.valuesByMode)[0];
    }
    return (cache[key] = val);
  }
  function v(name) { const x = vars[name]; if (!x) throw new Error('Variable fehlt: ' + name); return x; }
  async function farbe(name, deckkraft) {
    const x = v(name); const c = await wert(x, HELL);
    // Figma setzt die opacity einer gebundenen Farbe auf 1 zurück. Deckkraft
    // deshalb über node.opacity oder über eine Variable, deren Wert Alpha trägt.
    if (deckkraft !== undefined) throw new Error('CC.farbe: Deckkraft an gebundener Farbe wirkt nicht — node.opacity oder Variable mit Alpha nutzen');
    const p = { type: 'SOLID', color: { r: c.r, g: c.g, b: c.b } };
    return [figma.variables.setBoundVariableForPaint(p, 'color', x)];
  }
  const stile = {}; for (const s of await figma.getLocalTextStylesAsync()) stile[s.name.replace(/^Schrift\//, '')] = s;
  const effekte = {}; for (const s of await figma.getLocalEffectStylesAsync()) effekte[s.name.replace(/^Ebene\//, '')] = s;
  const schriften = new Set(Object.values(stile).map(s => JSON.stringify(s.fontName)));
  for (const f of ['IBM Plex Sans|Regular', 'IBM Plex Sans|Medium', 'IBM Plex Sans|SemiBold', 'IBM Plex Sans|Bold',
                   'IBM Plex Mono|Regular', 'IBM Plex Mono|Medium', 'Source Serif 4|Regular', 'Source Serif 4|SemiBold'])
    schriften.add(JSON.stringify({ family: f.split('|')[0], style: f.split('|')[1] }));
  await Promise.all([...schriften].map(f => figma.loadFontAsync(JSON.parse(f)).catch(() => null)));

  // Text mit Stil und Farbvariable. breite: Zahl = feste Breite mit Umbruch,
  // 'FILL' = füllt das Auto-Layout-Elternteil (nach dem Anhängen), sonst Hug.
  async function text(eltern, inhalt, stil, farbName, breite) {
    const t = figma.createText();
    if (!stile[stil]) throw new Error('Textstil fehlt: ' + stil);
    await t.setTextStyleIdAsync(stile[stil].id);
    t.characters = inhalt;
    t.fills = await farbe(farbName || 'text/stark');
    if (eltern) eltern.appendChild(t);
    if (typeof breite === 'number') { t.resize(breite, t.height); t.textAutoResize = 'HEIGHT'; }
    else if (breite === 'FILL') { t.layoutSizingHorizontal = 'FILL'; t.textAutoResize = 'HEIGHT'; }
    return t;
  }
  // Auto-Layout-Rahmen ohne Füllung.
  function box(eltern, name, richtung, abstand, props) {
    const f = figma.createAutoLayout(richtung || 'VERTICAL', Object.assign({ name, itemSpacing: abstand ?? 0 }, props || {}));
    f.fills = [];
    if (eltern) eltern.appendChild(f);
    return f;
  }
  // Innenabstand: pad(f, alle) | pad(f, senkrecht, waagerecht) | pad(f, oben, rechts, unten, links)
  function pad(f, a, b, c, d) {
    if (b === undefined) b = a; if (c === undefined) c = a; if (d === undefined) d = b;
    f.paddingTop = a; f.paddingRight = b; f.paddingBottom = c; f.paddingLeft = d; return f;
  }
  function radius(f, name) {
    const x = v('radius/' + name);
    for (const e of ['topLeftRadius', 'topRightRadius', 'bottomLeftRadius', 'bottomRightRadius']) f.setBoundVariable(e, x);
    return f;
  }
  // Nur eine Kante zeichnen: kante(f, 'unten', 'linie/normal')
  async function kante(f, seite, farbName, staerke) {
    f.strokes = await farbe(farbName || 'linie/normal');
    f.strokeWeight = staerke || 1;
    f.strokeTopWeight = seite === 'oben' ? (staerke || 1) : 0;
    f.strokeBottomWeight = seite === 'unten' ? (staerke || 1) : 0;
    f.strokeLeftWeight = seite === 'links' ? (staerke || 1) : 0;
    f.strokeRightWeight = seite === 'rechts' ? (staerke || 1) : 0;
    return f;
  }
  async function rand(f, farbName, staerke) { f.strokes = await farbe(farbName || 'linie/normal'); f.strokeWeight = staerke || 1; f.strokeAlign = 'INSIDE'; return f; }
  function ebene(f, name) { if (effekte[name]) f.effectStyleId = effekte[name].id; return f; }
  function luft(eltern) { const f = figma.createFrame(); f.name = 'Luft'; f.fills = []; eltern.appendChild(f); f.layoutSizingHorizontal = 'FILL'; f.layoutSizingVertical = 'FILL'; return f; }
  // Instanz aus einem Komponentensatz: variante(SATZ_ID, 'Art=Stark, Größe=Gross')
  async function variante(satzId, name, props) {
    const satz = await figma.getNodeByIdAsync(satzId);
    if (!satz) throw new Error('Komponente fehlt: ' + satzId);
    let quelle = satz;
    if (satz.type === 'COMPONENT_SET') {
      quelle = satz.children.find(c => c.name === name);
      if (!quelle) throw new Error('Variante fehlt: ' + name + ' in ' + satz.name + ' — vorhanden: ' + satz.children.map(c => c.name).join(' | '));
    }
    const i = quelle.createInstance();
    if (props) {
      const defs = satz.type === 'COMPONENT_SET' || (satz.type === 'COMPONENT' && satz.parent?.type !== 'COMPONENT_SET') ? satz.componentPropertyDefinitions : {};
      const setzen = {};
      for (const [k, wert] of Object.entries(props)) {
        const schluessel = Object.keys(defs).find(d => d === k || d.startsWith(k + '#'));
        if (!schluessel) throw new Error('Eigenschaft fehlt: ' + k + ' — vorhanden: ' + Object.keys(defs).join(', '));
        setzen[schluessel] = wert;
      }
      i.setProperties(setzen);
    }
    return i;
  }

  // Ikone als Instanz, optisch konstanter Strich je Größe (Richtung: Ikonen).
  const IKONEN = {"mic": "58:18", "mic-off": "58:31", "hand": "58:42", "undo-2": "58:51", "list-ordered": "58:69", "list-checks": "58:81", "files": "58:91", "file-text": "58:103", "ellipsis": "58:113", "lock": "58:127", "check": "58:135", "circle-check": "58:144", "x": "58:153", "minus": "58:161", "vote": "58:171", "circle-dot": "58:185", "circle-pause": "58:195", "wifi-off": "58:214", "wifi": "58:225", "refresh-cw": "58:236", "loader-circle": "58:244", "clock": "58:253", "calendar-clock": "58:266", "video-off": "58:281", "camera-off": "58:292", "cctv": "58:304", "user-x": "58:315", "armchair": "58:326", "captions": "58:340", "delete": "58:350", "chevron-right": "58:358", "triangle-alert": "58:368", "octagon-x": "58:378", "info": "58:388", "layout-grid": "58:404", "users": "58:415", "key-round": "58:424", "clipboard-check": "58:434", "activity": "58:442", "volume-2": "58:452", "stamp": "58:462", "share": "58:472", "printer": "58:487", "copy": "58:496", "grip-vertical": "58:509", "log-out": "58:519", "headphones": "58:527", "mic-offen": "58:544", "staffelstab": "58:553", "saal": "58:567", "totale": "58:578", "randstrich": "58:590", "vollmacht": "58:601", "nfc-tag": "58:612"};
  const STRICH = { 16: 1.5, 20: 1.75, 24: 1.75, 28: 2, 32: 2, 40: 2.5, 48: 3, 64: 3.5 };
  async function ikone(eltern, name, groesse, farbName) {
    const id = IKONEN[name]; if (!id) throw new Error('Ikone fehlt: ' + name + ' — vorhanden: ' + Object.keys(IKONEN).join(', '));
    const k = await figma.getNodeByIdAsync(id);
    const i = k.createInstance(); i.name = 'Ikone/' + name;
    const g = groesse || 24; i.resize(g, g);
    const strich = STRICH[g] || (1.75 * g / 24);
    const p = await farbe(farbName || 'text/stark');
    for (const v of i.findAll(n => n.type === 'VECTOR' || n.type === 'ELLIPSE' || n.type === 'RECTANGLE' || n.type === 'LINE' || n.type === 'BOOLEAN_OPERATION' || n.type === 'STAR' || n.type === 'POLYGON')) {
      if (Array.isArray(v.strokes) && v.strokes.length) { v.strokes = p; v.strokeWeight = strich; }
      if (Array.isArray(v.fills) && v.fills.length) v.fills = p;
    }
    if (eltern) eltern.appendChild(i);
    return i;
  }
  // Foto als Füllung (Symbolbild): bild(knoten, 'saal'|'platz'|'kamera'|'stimme'|'halle'|'server', 'FILL'|'FIT')
  const BILDER = {"saal": "c6cf4440c394da8f9c3999d144d8f31fe5cef3f1", "platz": "f04a4ebf708a0ceeded5c98e66ea643044e4f4f3", "kamera": "292c38227941f415996f83dbbbb5bc65493eef92", "stimme": "f6388b469b6b2df16a6ce5fabec0b655913bea76", "halle": "c319ef1047d3e2f7668f5852063f683d0bbc0465", "server": "1f552d263539b135ce95d647d07efd274277b46d"};
  function bild(knoten, name, modus) {
    if (!BILDER[name]) throw new Error('Bild fehlt: ' + name);
    knoten.fills = [{ type: 'IMAGE', imageHash: BILDER[name], scaleMode: modus || 'FILL' }];
    return knoten;
  }
  return { ikone, bild, IKONEN, vars, v, farbe, stile, effekte, text, box, pad, radius, kante, rand, ebene, luft, variante, HELL, DUNKEL, farbSammlung };
})();
