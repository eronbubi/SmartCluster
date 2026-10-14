# ADR-0013: Website: ZIP-Upload + Reverse Proxy

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Die Rolle „Website“ braucht Inhalt. Die Anforderung sagt nicht, woher er kommt.

## Entscheidung
v1 bietet zwei Modi, die sich kombinieren lassen.

**1. ZIP-Upload in der UI (statische Seiten)**
- **Grenzen:** höchstens 200 MB komprimiert, 1 GB entpackt, 20.000 Dateien.
- **Abgelehnt werden:** absolute Pfade, `..`, Symlinks, Gerätedateien (Schutz gegen Zip-Slip und Zip-Bomben).
- **Speicherung:** inhaltsadressiert (SHA-256) in der Kontrollzentrale.
- **Verteilung:** Die Website-Rolle bekommt den Parameter `content_sha256`. Bei `role.apply` lädt der Agent den Inhalt per mTLS von `GET /agent/v1/content/{sha256}`, prüft den Hash und entpackt in ein neues Verzeichnis. Danach schaltet er atomar um (Symlink-Wechsel), Rollback ist möglich.
- **Ausführung:** Caddy liefert nur aus (`file_server`). Serverseitiger Code (PHP, CGI) wird nicht ausgeführt.

**2. Reverse Proxy für andere Rollen**
- Der Member `entry` leitet per Subdomain oder Pfad auf HTTP-Dienste anderer Assignments weiter, etwa Nextcloud oder Open WebUI.
- Die UI bietet nur Assignments an, deren Template einen HTTP-Endpunkt deklariert.

**Später (nicht v1):** Git-URL.

## Konsequenzen
- **Hochgeladener Inhalt ist Daten, kein Code.** Die Regel „nur signierte Templates werden ausgeführt“ (ARCHITECTURE.md 5.4) bleibt gültig, weil nichts aus dem ZIP ausgeführt wird. JavaScript im ZIP läuft im Browser der Besucher, nicht auf dem Node.
- Neue Template-Fähigkeit `content: {param, target}`, neuer Agent-Endpunkt und eine Tabelle `content_blobs` in der DB.
- Der Speicher der Kontrollzentrale wächst mit jeder Version. Es werden die letzten 5 Versionen pro Assignment behalten.
- Bei mehreren Backends verteilt jedes `role.apply` denselben Hash, damit alle denselben Stand haben.

## Aufgaben
1. [ ] Upload-Endpunkt mit Prüfungen und Fuzz-Tests für ZIP-Parsing (Meilenstein c).
2. [ ] `entry`-Routing auf andere Assignments (Meilenstein c, Ziele aus d und e kommen dazu).
