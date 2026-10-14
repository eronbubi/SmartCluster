# ADR-0004: Cloud-Storage: Nextcloud + Garage statt MinIO

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Die Anforderung nennt „Nextcloud oder MinIO“. Zum Stand der MinIO-Community-Edition:
- Seit Oktober 2025 gibt es keine offiziellen Images mehr.
- Im Dezember 2025 ging das Projekt in den Wartungsmodus.
- Seit Februar 2026 ist das Repository archiviert.

Ein Produkt darf keine Komponente ausliefern, für die es keine Sicherheitsupdates mehr gibt.

## Entscheidung
- **Nextcloud** ist die Benutzer-Cloud: Dateien, Web-UI, Sync-Clients. Sie läuft in v1 nur auf einem Node, mit eigener Postgres- und Valkey-Instanz in derselben Rolle.
- **Garage** (AGPL-3.0) ist der S3-Speicher mit 2- oder 3-facher Replikation über mehrere Nodes. Jeder Node bildet eine eigene Zone.
- Beide laufen als unveränderte Container, das ist mit Closed Source vereinbar.

## Betrachtete Optionen

| Kriterium | Nextcloud + Garage | MinIO CE | SeaweedFS |
|---|---|---|---|
| Gepflegt | ja | archiviert | ja |
| Für kleine Cluster auf gemischter Hardware | Garage ist genau dafür gebaut | ab 4 Laufwerken (Erasure Coding) | ja, aber komplexer |
| Benutzeroberfläche für Einsteiger | Nextcloud | Konsole aus der CE entfernt | keine |

## Konsequenzen
- „Cloud-Storage“ besteht aus zwei Rollen. Die UI erklärt den Unterschied: „Meine Dateien (Nextcloud)“ und „S3-Speicher für Programme (Garage)“.
- Nextcloud auf mehreren Nodes kommt nicht in v1. Später kann Nextcloud Garage als Primärspeicher nutzen.
- Garage hat keine eigene Web-UI. SmartCluster legt einen Bucket und einen Access-Key an und zeigt beides an.

## Aufgaben
1. [ ] Rollen `storage-nextcloud` und `storage-s3` (Meilenstein d).
2. [ ] Verschlüsselung der Garage-internen RPC verifizieren (Meilenstein d).
