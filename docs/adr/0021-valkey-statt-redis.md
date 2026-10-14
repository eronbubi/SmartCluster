# ADR-0021: Valkey statt Redis

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Die Anforderung nennt Redis. Redis steht seit Version 7.4 unter RSAL/SSPL, seit Redis 8 zusätzlich unter AGPLv3. Valkey ist der Fork der Linux Foundation, steht unter BSD-3 und ist Redis-kompatibel.

## Entscheidung
Die Datenbank-Rolle „Redis“ läuft mit **Valkey**. In der UI heißt sie „Redis-kompatibel (Valkey)“. Auch Nextcloud nutzt intern Valkey.

## Konsequenzen
- Die Lizenz ist einfach (BSD-3), und es gibt keine Copyleft-Fragen.
- Clients und Protokoll sind Redis-kompatibel. Für Einsteiger ändert sich nichts.
- Die Replikation (Primary + Replica, manuelles Promote) funktioniert wie bei Redis.

## Aufgaben
1. [ ] Rolle `database-redis` auf Basis von `valkey/valkey` (Meilenstein d).
