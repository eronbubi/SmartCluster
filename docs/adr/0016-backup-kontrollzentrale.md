# ADR-0016: Tägliches Backup der Kontrollzentrale auf zweiten Node

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Stirbt die Platte der Kontrollzentrale, sind DB, Cluster-CA und Schlüssel weg. Dann muss der Cluster neu aufgesetzt und jeder Node neu gekoppelt werden.

## Entscheidung
**Täglich um 03:00 Uhr Ortszeit** erstellt die Kontrollzentrale ein Backup und verteilt es auf einen zweiten Node.

- **Inhalt:**
  - `pg_dump` im Custom-Format
  - Master-Key
  - Versionsinfo
  - Rollen-Assignments inklusive der Secrets, die in der DB ohnehin verschlüsselt liegen
- **Verschlüsselung:**
  - Das Backup wird mit einem zufälligen Datenschlüssel verschlüsselt (AES-256-GCM).
  - Dieser Datenschlüssel wird zweimal verpackt: mit dem Master-Key (für die Wiederherstellung am selben Ort) und mit dem **Wiederherstellungsschlüssel**.
  - Den Wiederherstellungsschlüssel bekommt der Admin einmalig im Onboarding: 24 Zeichen, zum Ausdrucken oder Abschreiben.
- **Ziel:**
  - Der Admin wählt den Ziel-Node. Standard ist der Node mit dem meisten freien Platz, außer der Kontrollzentrale selbst.
  - Der Agent des Ziels holt die Datei per mTLS (`GET /agent/v1/backups/{id}`) und legt sie unter `/var/lib/smartcluster/backups/` ab.
  - Es werden 7 Versionen behalten.
- **Einzelserver:** Ohne zweiten Node bleibt das Backup lokal. Dazu kommt eine dauerhafte Warnung und ein Download-Knopf in der UI.
- **Wiederherstellung:** `smartcluster-agent init --restore <datei>` auf einem neuen Rechner, danach die Eingabe des Wiederherstellungsschlüssels.

## Konsequenzen
- Neuer Wizard-Schritt „Wiederherstellungsschlüssel sichern“, mit Bestätigung, dass er notiert ist (Meilenstein b).
- **Ist der Wiederherstellungsschlüssel verloren und die Kontrollzentrale tot, ist das Backup wertlos.** Die UI sagt das genau so.
- Da die CA wiederhergestellt wird, vertrauen die Agents der neuen Kontrollzentrale ohne Neukopplung. Hat sie eine neue IP, finden die Agents sie per mDNS und prüfen die gepinnte CA.
- Neue Befehle bzw. Endpunkte: `backup.store {id, sha256}` und `GET /agent/v1/backups/{id}`.
- Die Wiederherstellung wird in CI automatisch getestet: Backup → leere Kontrollzentrale → Restore → Agents verbinden sich.

## Aufgaben
1. [ ] Wiederherstellungsschlüssel im Onboarding (Meilenstein b).
2. [ ] Backup-Job, Verteilung, Restore-Pfad mit CI-Test (Meilenstein d).
