# ADR-0020: Kontroll-App für Windows + Linux

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Die Kontroll-App läuft auf dem Haupt-PC des Nutzers.

## Entscheidung
Die Electron-App gibt es für **Windows x64** und **Linux x64** (AppImage und .deb). macOS kommt später. Die Web-UI funktioniert in jedem aktuellen Browser, auch auf macOS.

## Konsequenzen
- macOS-Nutzer verwenden bis dahin die Web-UI, mit CA-Import (ADR-0015) und Remote-Desktop über einen selbst installierten RustDesk-Client.
- Später braucht macOS ein Apple-Developer-Konto für Signatur und Notarisierung (Kosten).
- Der USB-Stick-Ersteller (geführte Linux-Installation) braucht Rohzugriff auf Datenträger, unter Windows und Linux jeweils mit erhöhten Rechten.

## Aufgaben
1. [ ] electron-builder-Ziele für Windows und Linux (Meilenstein a für Dev-Builds, g für Installer).
