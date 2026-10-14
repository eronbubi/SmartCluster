# ADR-0010: Lizenzmodell: Offline-Keys, Free/Home/Pro, 14-Tage-Trial

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Das Produkt ist selbstgehostet und muss offline funktionieren. Für eine Online-Aktivierung bräuchte es einen Lizenzserver des Herstellers, also eine zusätzliche Komponente.

## Entscheidung
**Offline signierte Lizenzschlüssel** (Ed25519, Format `SC1.<payload>.<sig>`), geprüft gegen den einkompilierten öffentlichen Schlüssel. Online-Aktivierung kommt später und wird dann ein neues ADR.

| Edition | Nodes | AI Multi-Node | Remote-Desktop | Übrige Rollen | Schlüssel |
|---|---|---|---|---|---|
| Free | 1 | nein | nein | alle (Single-Node) | keiner nötig |
| Home | bis 5 | ja | ja | alle | `ed: "home"` |
| Pro | unbegrenzt | ja | ja | alle | `ed: "pro"` |
| Trial | 14 Tage ab Cluster-Erstellung | wie Home | wie Home | wie Home | keiner nötig |

**Annahme, bitte bestätigen:** Die Trial hat den Umfang von **Home**. Danach gelten ohne Schlüssel die Regeln von **Free**.

**Durchsetzung:**
- Laufende Dienste werden **nie** gestoppt.
- Bei abgelaufener Lizenz oder überschrittenem Umfang sind gesperrt: neue Nodes, neue Rollen-Zuweisungen auf Nodes über dem Limit, Remote-Desktop-Sessions und Multi-Node-AI-Zuweisungen.
- Anzeigen, Logs, Rollen entfernen und Neustart bleiben immer erlaubt.

## Betrachtete Optionen
- **Offline-Keys (gewählt):** keine Infrastruktur, funktioniert ohne Internet. Dafür lässt sich ein Key auf beliebig vielen Clustern nutzen.
- **Online-Aktivierung:** bindet Keys an Cluster, braucht aber Server, Datenschutz und Verfügbarkeit. Kommt später.

## Konsequenzen
- **Trial lässt sich zurücksetzen:** Der Startzeitpunkt liegt in der DB der Kontrollzentrale. Wer den Cluster neu aufsetzt, bekommt eine neue Trial. Das ist bis zur Online-Aktivierung akzeptiert.
- **Patchen bleibt möglich:** Wer das Binary patcht, umgeht jede Prüfung (ARCHITECTURE.md 1.13). Ziel ist, ehrliche Kunden einfach zu lizenzieren, nicht Kopierschutz.
- `tools/licensegen` läuft nur beim Hersteller. Der private Schlüssel ist nie im Repository.
- Free mit einem Node ist effektiv der Modus „Einzelserver“.

## Aufgaben
1. [ ] Lizenzprüfung, Durchsetzungspunkte in Engine und API, Trial-Zähler, `licensegen` (Meilenstein g).
2. [ ] Lizenzhinweise in der UI (Restlaufzeit der Trial, Limits) (Meilenstein g).
