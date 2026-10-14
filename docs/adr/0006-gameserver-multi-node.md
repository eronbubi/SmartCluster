# ADR-0006: Gameserver: unabhängige Server, optional Velocity

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Eine Minecraft-Welt läuft auf genau einem Rechner. Lastverteilung einer Welt über mehrere Rechner gibt es nicht, auch Folia verteilt nur auf die Kerne einer Maschine.

## Entscheidung
Mehrere Gameserver-Nodes bedeuten **mehrere unabhängige Server**, jeder mit eigener Welt. Optional verbindet ein **Velocity-Proxy** (Member `proxy`) sie zu einem Netzwerk: Spieler verbinden sich mit einer Adresse und wechseln per `/server`.

## Betrachtete Optionen
- **Unabhängig + optional Velocity (gewählt):** technisch ehrlich, und das Netzwerk-Erlebnis bleibt möglich.
- **Nur unabhängige Server:** einfacher, aber jeder Server braucht einen eigenen Port und eine eigene Adresse.
- **„Lastverteilung“ versprechen:** technisch falsch.

## Konsequenzen
- UI-Label: „Mehrere unabhängige Server“, mit Erklärtext (Rollen-Template `multiNode.explain`).
- Velocity braucht „modern forwarding“ mit einem gemeinsamen Secret. SmartCluster erzeugt es und verteilt es an alle Server im Netzwerk.
- Die EULA-Zustimmung bleibt pro Rolle Pflicht und wird nie automatisch gesetzt.

## Aufgaben
1. [ ] Rolle `gameserver-minecraft` mit den Membern `server` und `proxy` (Meilenstein c).
