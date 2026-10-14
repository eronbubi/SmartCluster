# ADR-0018: CI über GitHub Actions, Authenticode später

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Der Code ist Closed Source. Windows-Installer ohne Authenticode-Signatur lösen SmartScreen-Warnungen aus, und Virenscanner melden ungesignierte Go-Binaries häufig fälschlich.

## Entscheidung
- **CI:** GitHub Actions in einem privaten Repository. Die Jobs stehen in ARCHITECTURE.md 15.
- **Hersteller-Signaturen (Ed25519)** für Releases, Rollen und Lizenzen gibt es ab dem ersten Release-Build. Die Schlüssel liegen nur in einer geschützten Release-Umgebung mit manueller Freigabe.
- **Authenticode:** noch nicht vorhanden, kommt später.

## Konsequenzen
- Bis zum Authenticode-Zertifikat zeigt Windows „Unbekannter Herausgeber“. Die Installationsanleitung muss das erklären (Klick auf „Weitere Informationen → Trotzdem ausführen“). Für den Schritt zum zahlenden Kunden ist das nicht akzeptabel. **Vor dem ersten öffentlichen Release muss das Zertifikat da sein.**
- Private Repos haben ein begrenztes Kontingent an Actions-Minuten, Windows-Runner zählen doppelt. Windows-Jobs laufen deshalb nur bei Änderungen an `/agent` und bei Releases.
- Updates bleiben auch ohne Authenticode sicher, weil der Agent die Ed25519-Signatur prüft.

## Aufgaben
1. [ ] Workflows `go`, `app`, `contracts`, `roles` (Meilenstein a), `integration`, `e2e` (ab a bzw. b), `release` (Meilenstein g).
2. [ ] Authenticode-Zertifikat beschaffen, bevor die erste Version extern ausgeliefert wird.
