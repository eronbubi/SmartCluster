# ADR-0007: Registrierung: Setup-Code + Einladungen

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Wenn sich auf einem privaten Cluster jeder registrieren kann, kann jeder im LAN Admin werden. „Passwort vergessen“ per E-Mail bräuchte SMTP, und das steht nicht in der Anforderung.

## Entscheidung
- Der **erste Account** wird Admin. Die Registrierung braucht den **Setup-Code**, den die Kontrollzentrale nach der Installation anzeigt (20 Zeichen, 100 Bit). Der Code authentifiziert zugleich die Kontrollzentrale gegenüber der App (Kanalbindung, ARCHITECTURE.md 4.3).
- **Weitere Accounts** gibt es nur per Einladungslink von einem Admin: einmalig gültig, 7 Tage, Rolle schon festgelegt.
- **Passwort zurücksetzen:** Ein Admin erzeugt einen Einmal-Link. Der Admin selbst setzt sein Passwort per CLI auf dem Rechner der Kontrollzentrale zurück (root bzw. Admin nötig).

## Betrachtete Optionen
- **Setup-Code + Einladungen (gewählt):** sicher, ohne SMTP.
- **Offene Registrierung:** unsicher.
- **Wer zuerst kommt, wird Admin:** Race-Condition im LAN.

## Konsequenzen
- Wer den Setup-Code verliert, bevor der erste Account existiert, liest ihn erneut mit `smartcluster-agent status` auf dem Rechner der Kontrollzentrale aus.
- Einladungslinks enthalten Tokens. Sie werden nur als Hash gespeichert und nie geloggt.

## Aufgaben
1. [ ] `/setup/*`, `/auth/invitations/*` und die Admin-Reset-CLI (Meilenstein b).
