# ADR-0009: Node-Kopplung: Join-Code + mDNS, kein QR

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Die Anforderung nennt „QR/Token“. Ein QR-Code auf dem Bildschirm des Haupt-PCs müsste vom alten Laptop gescannt werden. Dem fehlt meist eine passende Kamera oder Software.

## Entscheidung
- **Join-Code:**
  - 20 Zeichen Crockford-Base32 (100 Bit)
  - gilt 30 min und einmalig
  - angezeigt als `XXXXX-XXXXX-XXXXX-XXXXX`
  - Gespeichert wird nur ein mit HKDF abgeleiteter Schlüssel.
- **Kanalbindung:** Der Code schützt das Enrollment gegen Man-in-the-Middle (ARCHITECTURE.md 4.3).
- **Suche:** Der Agent findet die Kontrollzentrale per mDNS (`_smartcluster._tcp`). Als Fallback gibt man die Adresse ein.
- Ein **QR-Code entfällt**.

## Konsequenzen
- 20 Zeichen zu tippen ist zumutbar, Verwechslungen fängt Crockford-Base32 ab (I/L → 1, O → 0).
- mDNS funktioniert nicht über VLAN- oder Subnetzgrenzen. Dort hilft der Fallback mit der Adresse.
- Bei der geführten Linux-Installation steckt der Code bereits im USB-Stick.

## Aufgaben
1. [ ] Code-Erzeugung per CLI (Meilenstein a) und in der UI (Meilenstein b), Enrollment-Protokoll mit Tests gegen MITM (Meilenstein a).
