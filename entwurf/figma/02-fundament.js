// Phase 2b — Fundamentseite: Farbtafeln in beiden Modi, Schriftmuster,
// Abstandsleiter. Läuft gegen Datei cKPTYwkOrRq2nsy1oyiUx2.
const V = async (id) => await figma.variables.getVariableByIdAsync(id);
const fuellung = async (id) => [figma.variables.setBoundVariableForPaint(
  { type: 'SOLID', color: { r: 0, g: 0, b: 0 } }, 'color', await V(id))];

const seite = await figma.getNodeByIdAsync('4:2');           // Fundament
await figma.setCurrentPageAsync(seite);

for (const [fam, sch] of [['IBM Plex Sans','Regular'], ['IBM Plex Sans','SemiBold'],
                          ['IBM Plex Sans','Medium'], ['Source Serif 4','SemiBold'],
                          ['Source Serif 4','Regular'], ['IBM Plex Mono','Medium'],
                          ['IBM Plex Mono','Regular']]) {
  await figma.loadFontAsync({ family: fam, style: sch });
}

const farbe = (await figma.variables.getLocalVariableCollectionsAsync())
  .find(c => c.name === 'Farbe');
const hell = farbe.modes.find(m => m.name === 'Hell').modeId;
const dunkel = farbe.modes.find(m => m.name === 'Dunkel').modeId;

const erzeugt = [];

async function ueberschrift(eltern, text, stil) {
  const t = figma.createText();
  t.fontName = stil || { family: 'IBM Plex Sans', style: 'SemiBold' };
  t.characters = text;
  t.fontSize = 19;
  t.fills = await fuellung('VariableID:2:30');
  eltern.appendChild(t);
  return t;
}

// ---------- Rahmen ----------
const blatt = figma.createFrame();
blatt.name = 'Fundament';
blatt.resize(1600, 1400);
blatt.x = 0; blatt.y = 0;
blatt.fills = await fuellung('VariableID:2:25');
seite.appendChild(blatt);
erzeugt.push(blatt.id);

const spalte = figma.createAutoLayout('VERTICAL', { name: 'Inhalt', itemSpacing: 48 });
blatt.appendChild(spalte);
spalte.x = 64; spalte.y = 64;
spalte.fills = [];

// ---------- Farben, beide Modi nebeneinander ----------
const farbBlock = figma.createAutoLayout('VERTICAL', { name: 'Farben', itemSpacing: 16 });
spalte.appendChild(farbBlock); farbBlock.fills = [];
await ueberschrift(farbBlock, 'Farben');

const semantik = [
  ['flaeche/grund', 'VariableID:2:25'], ['flaeche/blatt', 'VariableID:2:26'],
  ['flaeche/vertieft', 'VariableID:2:27'], ['linie/normal', 'VariableID:2:28'],
  ['text/stark', 'VariableID:2:30'], ['text/leise', 'VariableID:2:31'],
  ['marke/haus', 'VariableID:2:33'], ['signal/mikro', 'VariableID:2:34'],
  ['signal/wort', 'VariableID:2:35'], ['signal/frei', 'VariableID:2:36'],
  ['signal/achtung', 'VariableID:2:37']
];

for (const [modusName, modusId] of [['Hell', hell], ['Dunkel', dunkel]]) {
  const reihe = figma.createAutoLayout('VERTICAL', { name: modusName, itemSpacing: 8 });
  farbBlock.appendChild(reihe);
  reihe.fills = await fuellung('VariableID:2:26');
  reihe.setExplicitVariableModeForCollection(farbe, modusId);
  reihe.paddingTop = 16; reihe.paddingBottom = 16;
  reihe.paddingLeft = 16; reihe.paddingRight = 16;
  reihe.cornerRadius = 8;

  const kopf = figma.createText();
  kopf.fontName = { family: 'IBM Plex Sans', style: 'SemiBold' };
  kopf.characters = modusName.toUpperCase();
  kopf.fontSize = 11;
  kopf.letterSpacing = { unit: 'PIXELS', value: 1.2 };
  kopf.fills = await fuellung('VariableID:2:31');
  reihe.appendChild(kopf);

  const tafeln = figma.createAutoLayout('HORIZONTAL', { name: 'Tafeln', itemSpacing: 12 });
  reihe.appendChild(tafeln); tafeln.fills = [];
  for (const [name, id] of semantik) {
    const zelle = figma.createAutoLayout('VERTICAL', { name, itemSpacing: 6 });
    tafeln.appendChild(zelle); zelle.fills = [];
    const feld = figma.createFrame();
    feld.resize(104, 72);
    feld.cornerRadius = 4;
    feld.fills = await fuellung(id);
    feld.strokes = await fuellung('VariableID:2:28');
    feld.strokeWeight = 1;
    zelle.appendChild(feld);
    const bez = figma.createText();
    bez.fontName = { family: 'IBM Plex Mono', style: 'Regular' };
    bez.characters = name;
    bez.fontSize = 11;
    bez.fills = await fuellung('VariableID:2:31');
    zelle.appendChild(bez);
  }
}

// ---------- Schriftmuster ----------
const schriftBlock = figma.createAutoLayout('VERTICAL', { name: 'Schrift', itemSpacing: 20 });
spalte.appendChild(schriftBlock); schriftBlock.fills = [];
await ueberschrift(schriftBlock, 'Schrift');

const hinweis = figma.createText();
hinweis.fontName = { family: 'IBM Plex Sans', style: 'Regular' };
hinweis.resize(760, 10);
hinweis.textAutoResize = 'HEIGHT';
hinweis.characters = 'Die Zuordnung trägt eine Aussage: Source Serif steht ausschließlich ' +
  'für Texte, die rechtlich binden — Beschluss und Tagesordnungspunkt. Plex Sans bedient, ' +
  'Plex Mono zählt. Wer im Protokoll eine Serife sieht, weiß, dass dort etwas Verbindliches steht.';
hinweis.fontSize = 14;
hinweis.lineHeight = { unit: 'PIXELS', value: 22 };
hinweis.fills = await fuellung('VariableID:2:31');
schriftBlock.appendChild(hinweis);

const muster = [
  ['Schrift/Beschluss', 'Source Serif 4', 'SemiBold', 40, 'Der Haushalt 2027 wird beschlossen.'],
  ['Schrift/Punkt', 'Source Serif 4', 'SemiBold', 24, 'TOP 3 — Genehmigung des Protokolls'],
  ['Schrift/Titel', 'IBM Plex Sans', 'SemiBold', 28, 'Vorstandssitzung'],
  ['Schrift/Fließtext', 'IBM Plex Sans', 'Regular', 16, 'Das Mikrofon geht auf, sobald die Leitung das Wort erteilt.'],
  ['Schrift/Zahl gross', 'IBM Plex Mono', 'Medium', 48, '7 von 11'],
  ['Schrift/Zahl', 'IBM Plex Mono', 'Medium', 20, 'Platz 07 · 00:14:32']
];
for (const [name, fam, sch, gr, text] of muster) {
  const zeile = figma.createAutoLayout('VERTICAL', { name, itemSpacing: 4 });
  schriftBlock.appendChild(zeile); zeile.fills = [];
  const bez = figma.createText();
  bez.fontName = { family: 'IBM Plex Mono', style: 'Regular' };
  bez.characters = name + ' · ' + fam + ' ' + sch + ' ' + gr;
  bez.fontSize = 11;
  bez.fills = await fuellung('VariableID:2:31');
  zeile.appendChild(bez);
  const probe = figma.createText();
  probe.fontName = { family: fam, style: sch };
  probe.characters = text;
  probe.fontSize = gr;
  probe.fills = await fuellung('VariableID:2:30');
  zeile.appendChild(probe);
}

// ---------- Abstandsleiter ----------
const abBlock = figma.createAutoLayout('VERTICAL', { name: 'Abstände', itemSpacing: 10 });
spalte.appendChild(abBlock); abBlock.fills = [];
await ueberschrift(abBlock, 'Abstände');
for (const [name, wert] of [['2xs',2],['xs',4],['sm',8],['md',12],['lg',16],
                            ['xl',24],['2xl',32],['3xl',48],['4xl',64]]) {
  const z = figma.createAutoLayout('HORIZONTAL', { name: 'abstand/' + name, itemSpacing: 12 });
  abBlock.appendChild(z); z.fills = []; z.counterAxisAlignItems = 'CENTER';
  const bez = figma.createText();
  bez.fontName = { family: 'IBM Plex Mono', style: 'Regular' };
  bez.characters = ('abstand/' + name).padEnd(14) + String(wert).padStart(3);
  bez.fontSize = 12;
  bez.fills = await fuellung('VariableID:2:31');
  z.appendChild(bez);
  const balken = figma.createFrame();
  balken.resize(wert, 14);
  balken.fills = await fuellung('VariableID:2:33');
  balken.cornerRadius = 2;
  z.appendChild(balken);
}

return { createdNodeIds: erzeugt, blatt: blatt.id, hoehe: blatt.height };
