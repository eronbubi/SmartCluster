# ADR-0008: Rechte: User nur lesend

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Die Anforderung nennt die Rollen User und Admin, legt die Rechte aber nicht fest.

## Entscheidung
- **User** darf nur lesen: Dashboard, Stats, Logs, Jobs, Cluster-Ansicht, Hardware-Check-Ergebnisse.
- **Admin** darf alles: Aktionen, Rollen, Nodes, Remote-Desktop, Secrets anzeigen, Benutzer, Lizenz, Audit-Log.

Die vollständige Matrix steht in ARCHITECTURE.md 5.5.

## Betrachtete Optionen
- **User read-only (gewählt):** einfach, sicher, leicht zu erklären.
- **Feingranulare Rechte pro Rolle oder Node:** steht nicht in der Anforderung und wäre für v1 zu viel.

## Konsequenzen
- Jede schreibende Route prüft `role == admin`, getestet über eine Tabelle aller Routen.
- Live-Logs sind für User sichtbar. Sie können Daten aus Anwendungen enthalten. Das ist akzeptiert, weil User vertrauenswürdige Mitbenutzer sind.

## Aufgaben
1. [ ] Autorisierungs-Middleware und Routentabellen-Test (Meilenstein b).
