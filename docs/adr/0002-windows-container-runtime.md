# ADR-0002: Linux-Container auf Windows-Nodes

**Status:** Akzeptiert. Desk-Spike positiv, Praxistest auf echter Hardware offen (siehe „Spike-Ergebnis“)
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Alle v1-Rollen brauchen Linux-Container. Ein Windows-Node muss sie nach einem Neustart **ohne angemeldeten Benutzer** starten, sonst ist er kein Server.
- **Docker Desktop:** braucht einen angemeldeten Benutzer und ist für Firmen ab 250 Mitarbeitern oder 10 Mio. USD Umsatz kostenpflichtig.
- **Docker Engine als Windows-Dienst:** kann nur Windows-Container.

## Entscheidung
**Weg B** wird nach erfolgreichem Praxistest umgesetzt: Docker Engine läuft in einer eigenen WSL2-Distro, die beim Booten ohne Login startet. **Fallback A** ist Docker Desktop mit automatischer Anmeldung eines eigenen Benutzers und sofortiger Bildschirmsperre. Der Wizard empfiehlt für Server-Rollen grundsätzlich Linux.

## Spike-Ergebnis (Desk-Recherche, 2026-10-14)
Belegt aus dem WSL-Quellcode (`microsoft/WSL`, master):
1. **WSL verweigert LocalSystem hart.** `LxssUserSessionFactory.cpp` wirft `WSL_E_LOCAL_SYSTEM_NOT_SUPPORTED`, Meldung „Running WSL as local system is not supported.“ Der Agent-Dienst, der als LocalSystem läuft, kann WSL also **nicht direkt** starten.
2. **Session 0 mit normalem Benutzer ist ausdrücklich vorgesehen.** WSL-Sitzungen werden pro Benutzer-SID geführt. Im Quelltext steht: „Sessions launched from session zero will only be terminated when the service is stopped.“ Damit überlebt eine Sitzung, die ein eigener Benutzer in Session 0 startet, An- und Abmeldungen anderer Benutzer.
3. **Standard-Leerlauftimeouts:** `wsl2.vmIdleTimeout` = 60 s, `general.instanceIdleTimeout` = 15 s. Es braucht einen dauerhaft laufenden `wsl.exe`-Prozess und zusätzlich `-1` in `.wslconfig`.
4. **`wsl --install <Distro> --name <Name> --location <Pfad> --no-launch --web-download`** ist in der aktuellen WSL-CLI vorhanden. Eine Distro lässt sich damit ohne Store und ohne Interaktion anlegen.
5. **Netzwerk:**
   - Windows 11 22H2+: `networkingMode=mirrored`. Eingehender Verkehr braucht eine Hyper-V-Firewall-Regel (VM-Creator-ID `{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}`).
   - Windows 10: Nur NAT ist verfügbar, eine Portweiterleitung (`netsh interface portproxy`) ist nötig.

Offene Punkte, die nur ein Praxistest klärt:
- Funktioniert Batch-Logon (Aufgabenplanung beim Systemstart) für WSL?
- Ist die Verweigerung der interaktiven Anmeldung für das Dienstkonto verträglich? Ein Microsoft-Q&A-Bericht von 2026 nennt den Fehler „user has not been granted the requested logon type“.
- Ist der Container aus dem LAN und aus dem SYSTEM-Kontext erreichbar?
- Läuft der Container nach Abmelden eines anderen Benutzers weiter?

Das Testkit liegt in `/tools/spikes/windows-docker-wsl/`.

## Design (Weg B)
```
Agent-Dienst (LocalSystem)
  ├─ legt bei Installation an: lokaler Benutzer "smartcluster-engine" (kein Admin,
  │  zufälliges Passwort, nur in der Aufgabenplanung/LSA gespeichert, vom Anmeldebildschirm versteckt)
  ├─ Aufgabe "SmartCluster Engine": beim Systemstart, als smartcluster-engine, Batch-Logon
  │     └─ smartcluster-agent.exe engine-keeper   (Session 0)
  │           ├─ wsl.exe -d smartcluster-engine -u root -- sleep infinity   (hält VM/Instanz am Leben)
  │           └─ Named Pipe \\.\pipe\smartcluster-engine (ACL: nur SYSTEM) für feste Operationen:
  │              Hook aus geprüftem Bundle ausführen, Status, Neustart der Distro
  └─ Docker-API: dockerd lauscht in der Distro auf 127.0.0.1:2376 mit mTLS (lokale Zertifikate);
     erreichbar über mirrored loopback (Win 11) bzw. die WSL-IP (Win 10)
Distro smartcluster-engine: Debian, systemd=true, docker.io, automount=false, interop=false
```
**Härtung:**
- `automount=false` und `interop=false`: Die Distro sieht keine Windows-Laufwerke und kann keine Windows-Programme starten.
- Das Dienstkonto hat keine Admin-Rechte.
- Ein Docker-API-Port ohne Authentifizierung ist verboten.

**Arbeitsspeicher:** WSL2 nimmt standardmäßig 50 % des RAMs. Der Agent setzt `memory` in `.wslconfig` auf Gesamt-RAM minus 1,5 GB.

## Betrachtete Optionen

| Kriterium | B: Engine in WSL2 | A: Docker Desktop + Auto-Login | C: Windows ohne Workload-Rollen |
|---|---|---|---|
| Läuft ohne Login | ja (Desk-Spike), Praxistest offen | nur über Auto-Login | – |
| Lizenzkosten für Kunden | keine (Apache-2.0) | ab 250 MA / 10 Mio. USD | keine |
| Sicherheit | eigenes Konto ohne Admin, Distro abgeschottet | Desktop-Sitzung offen bis zur Sperre, Passwort für Auto-Login gespeichert | am besten |
| Komplexität | hoch (Keeper, Netzwerk je Windows-Version) | mittel | gering |
| Nutzen für Einsteiger | voll | voll, mit Kompromiss | eingeschränkt |

## Konsequenzen
- Windows-Nodes brauchen eingeschaltetes VT-x/AMD-V. Der Hardware-Check macht das unter Windows zur Pflicht.
- Windows 10 und Windows 11 brauchen unterschiedliche Netzwerkpfade (portproxy bzw. mirrored). Das bedeutet doppelte Tests.
- Hooks laufen unter Windows über die Keeper-Pipe, nicht direkt aus dem Agent.
- WSL-Updates über den Store können die Distro neu starten. Der Keeper muss das erkennen und neu starten.

## Aufgaben
1. [ ] Praxistest mit dem Spike-Kit auf Windows 11 (Pflicht) und Windows 10 (optional) durchführen und Bericht auswerten.
2. [ ] Bei Erfolg: ADR-Status auf „Akzeptiert (verifiziert)“, `engine-keeper` in Meilenstein c umsetzen.
3. [ ] Bei Misserfolg: Ursache dokumentieren, Fallback A in neuem ADR festhalten.
