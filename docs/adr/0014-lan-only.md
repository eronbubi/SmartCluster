# ADR-0014: v1 nur im LAN erreichbar

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Damit Dienste aus dem Internet erreichbar sind, braucht es eine Portfreigabe im Router. Bei CGNAT oder DS-Lite ist das unmöglich. Tunnel-Dienste stehen nicht in der Anforderung.

## Entscheidung
SmartCluster v1 richtet nichts für den Internetzugang ein: kein UPnP, keine Tunnel. Die UI enthält eine **Anleitung zur Portfreigabe** mit dem Hinweis auf CGNAT und einem Test „Ist meine Verbindung CGNAT?“, der die WAN-IP des Routers mit der öffentlichen IP vergleicht, nur falls der Nutzer die WAN-IP selbst eingibt.

## Konsequenzen
- Website und Minecraft sind standardmäßig nur im Heimnetz erreichbar.
- Für die Kontrollzentrale und RustDesk gibt es bewusst keine Anleitung zum Freigeben. Die Ports 8443, 8444 und 21115–21119 gehören nicht ins Internet. Die UI warnt ausdrücklich davor.
- Tunnel-Lösungen kommen später als eigenes Feature mit eigenem ADR.

## Aufgaben
1. [ ] Anleitungstexte (de/en) in der Rollenansicht (Meilenstein c).
