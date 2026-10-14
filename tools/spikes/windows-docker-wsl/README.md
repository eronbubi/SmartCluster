# Spike 1.1: Docker Engine in WSL2 ohne Login (ADR-0002)

Wegwerf-Code. Er prüft, ob Weg B aus ADR-0002 auf echter Hardware funktioniert, bevor der Agent darauf gebaut wird.

## Hypothesen
| # | Hypothese | Wie geprüft |
|---|---|---|
| H1 | Eine WSL2-Distro startet beim Booten in Session 0 unter einem eigenen Konto ohne Admin-Rechte, **ohne dass sich jemand anmeldet** | Neustart ohne Anmeldung, `status.json` |
| H2 | dockerd und der Container laufen stabil weiter, auch im Leerlauf über 30 min | Heartbeat im `keeper.log` |
| H3 | Der Container ist aus dem LAN erreichbar | Handy oder anderer PC: `http://<IP>:8080` |
| H4 | Der Container läuft weiter, wenn sich ein anderer Benutzer an- und wieder abmeldet | manuell, siehe Ablauf Schritt 5 |
| H5 | Der Container ist aus dem SYSTEM-Konto erreichbar, also auf dem Weg, den später der Agent-Dienst nimmt | Aufgabe `SystemCheck` → `check-system.json` |
| H6 | Das Verweigern der interaktiven Anmeldung für das Engine-Konto stört WSL nicht | Standard-Lauf; bei Fehlern zum Vergleich mit `-AllowInteractiveLogon` |

**Vorab belegt aus dem WSL-Quellcode** (siehe ADR-0002):
- LocalSystem wird verweigert.
- Sessions aus Session 0 sind vorgesehen und enden erst, wenn der Dienst stoppt.
- Leerlauftimeouts: 60 s für die VM, 15 s für die Instanz.

## Was der Spike auf dem Rechner verändert
- WSL wird installiert, falls es fehlt. **Das braucht einen Neustart**, danach `setup.ps1` erneut ausführen.
- Lokales Konto `sc-engine`: kein Admin, zufälliges Passwort, das nur die Aufgabenplanung speichert, vom Anmeldebildschirm versteckt. Rechte: „Anmelden als Batchauftrag“. Interaktive und RDP-Anmeldung sind verweigert.
- Aufgaben unter `\SmartClusterSpike\`: `Engine` (als `sc-engine`), `SystemCheck` (als SYSTEM), `PortProxy` (als SYSTEM, nur Windows 10).
- Firewall: TCP 8080 nur für das lokale Subnetz. Unter Windows 11 kommt eine Hyper-V-Firewall-Regel für WSL dazu.
- `C:\ProgramData\SmartClusterSpike\` mit Skripten, Logs und der Debian-Distro (etwa 1,5 GB).
- Downloads aus offiziellen Quellen: WSL-Update (Microsoft), Debian (WSL-Katalog), `docker.io` (Debian-Pakete), `traefik/whoami` (Docker Hub).

`cleanup.ps1` entfernt alles wieder. WSL selbst bleibt installiert.

## Voraussetzungen
- **Testrechner**, bevorzugt Windows 11 22H2 oder neuer. Windows 10 22H2 läuft mit `-Networking nat`.
- Virtualisierung im BIOS eingeschaltet.
- Internet für den ersten Lauf.
- Ein zweites Gerät im selben Netz, zum Beispiel ein Handy.

## Ablauf
1. Den Ordner auf den Testrechner kopieren, **PowerShell als Administrator** öffnen und ausführen:
   ```powershell
   Set-ExecutionPolicy -Scope Process Bypass
   .\setup.ps1
   ```
   Meldet das Skript „WSL was installed“: Neustart, anmelden, `.\setup.ps1` noch einmal ausführen.
2. Das Skript wartet, bis der Container läuft. Der erste Lauf dauert 5–15 min, weil Debian, Docker und das Image heruntergeladen werden. Danach zeigt es das Zwischenergebnis und die URL.
3. Auf dem Handy die angezeigte URL öffnen, etwa `http://192.168.1.20:8080/`. Es erscheint eine Textseite mit `Hostname: …`.
4. **Neu starten und NICHT anmelden.** 5 min warten, dann die URL erneut auf dem Handy öffnen (H1, H3).
5. Anmelden. Die URL auf dem Handy prüfen, dann **abmelden** (nicht herunterfahren) und erneut prüfen (H4). Danach wieder anmelden.
6. PowerShell als Administrator öffnen:
   ```powershell
   Set-ExecutionPolicy -Scope Process Bypass
   C:\ProgramData\SmartClusterSpike\bin\check.ps1
   ```
   Den Bericht `C:\ProgramData\SmartClusterSpike\report-*.txt` zurückschicken. Dazu das Ergebnis von Schritt 3–5 (ja/nein je Schritt) notieren.
7. Optional: 30 min nichts tun, dann URL erneut prüfen (H2).
8. Aufräumen, ebenfalls als Administrator:
   ```powershell
   .\cleanup.ps1
   ```
   Achtung: Der Neustart von `WslService` beendet alle laufenden WSL-Distros auf diesem Rechner.

## Bei Fehlern
- Im Log steht „logon type“ oder „not granted“: `cleanup.ps1` ausführen, dann `setup.ps1 -AllowInteractiveLogon`. So lässt sich H6 isolieren.
- `phase: error` mit „Distro installation failed“: Steht `--name` oder `--location` nicht in `wsl --help`, ist die WSL-Version zu alt. Dann `wsl --update` als Administrator ausführen.
- Container lokal erreichbar, aber nicht vom Handy: Netzwerkprofil (öffentlich oder privat) und Router (Client-Isolation im Gast-WLAN) prüfen.
