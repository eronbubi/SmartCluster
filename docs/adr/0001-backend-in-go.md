# ADR-0001: Backend der Kontrollzentrale in Go

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Die Anforderung lässt für die Kontrollzentrale Node.js (TypeScript) oder Go offen. Der Agent ist bereits in Go festgelegt. Die Kontrollzentrale läuft auf alten Laptops, oft zusammen mit Workload-Rollen. Das Produkt ist Closed Source und enthält eine Lizenzprüfung.

## Entscheidung
Die Kontrollzentrale wird in **Go** geschrieben. Die Kontroll-App bleibt React/TypeScript. API-Typen für TypeScript werden aus `/docs/api/openapi.yaml` generiert.

## Betrachtete Optionen

| Kriterium | Go | Node.js (TS) |
|---|---|---|
| Gemeinsamer Code mit Agent (Protokoll, Signaturen, Templates) | ja, über `/pkg` | nein, doppelte Implementierung |
| RAM im Leerlauf | gering | höher (V8) |
| Auslieferung | ein statisches Binary | Runtime + `node_modules` |
| Schutz gegen Patchen der Lizenzprüfung | kompiliert, schwerer | JS-Bundle trivial lesbar |
| Typen zwischen Frontend und Backend | generiert aus OpenAPI | direkt teilbar |

## Konsequenzen
- Protokolltypen, Signaturprüfung und Template-Parser existieren nur einmal (`/pkg`).
- Frontend und Backend teilen keine Typen direkt. Der CI-Job `contracts` prüft, dass der generierte Code aktuell ist.
- Es entsteht keine Node.js-Laufzeitabhängigkeit auf den Nodes.

## Aufgaben
1. [ ] Go-Workspace (`go.work`) mit `agent`, `control`, `pkg` anlegen (Meilenstein a).
2. [ ] Codegen-Pipeline OpenAPI → Go (oapi-codegen) und → TS (openapi-typescript) (Meilenstein a).
