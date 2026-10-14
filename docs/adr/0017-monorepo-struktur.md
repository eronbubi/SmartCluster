# ADR-0017: Monorepo-Struktur mit `/pkg`, `/tools`, `/.github`

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Die Anforderung nennt `/agent`, `/control`, `/app`, `/roles` und `/docs`. Agent und Kontrollzentrale teilen Go-Code. Herstellerwerkzeuge dürfen nie ausgeliefert werden. CI braucht Workflow-Dateien.

## Entscheidung
```
/agent    Go-Modul: Agent
/control  Go-Modul: Kontrollzentrale
/pkg      Go-Modul: gemeinsamer Code (proto, sign, template, version)
/app      Electron + React (pnpm)
/roles    Rollen-Templates
/tools    Herstellerwerkzeuge (licensegen, rolesign, releasesign) und Spikes (tools/spikes/*), nie ausgeliefert
/docs     OpenAPI, WS-Schemas, ADRs, Benutzer-Doku
/.github  CI-Workflows
```
Die drei Go-Module sind über `go.work` verbunden.

## Konsequenzen
- `/tools` wird von keinem Release-Artefakt eingebunden. Ein CI-Check stellt sicher, dass `agent` und `control` nichts aus `/tools` importieren.
- Spikes sind Wegwerf-Code und unterliegen nicht den Qualitätsregeln für Produktcode.

## Aufgaben
1. [ ] Grundgerüst beim Start von Meilenstein a.
