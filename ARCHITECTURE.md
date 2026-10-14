# SmartCluster – Architektur

**Status:** Freigegeben · 2026-10-14 · Entscheidungen in [`docs/adr`](docs/adr/README.md)
**Vertraulich:** proprietär, Closed Source

Dieses Dokument legt Komponenten, Datenmodell, API, WebSocket-Protokoll und Rollen-Template-Format fest. Abschnitt 1 listet alles, was **technisch nicht so geht wie in der Anforderung beschrieben**, jeweils mit realistischer Alternative. Abschnitt 17 verweist auf die getroffenen Entscheidungen (ADRs).

---

## 0. Kurzfassung

- Jeder Rechner bekommt den **Agent** (Go, ein Binary, Systemdienst). Der Agent baut nur **ausgehende** mTLS-WebSocket-Verbindungen zur Kontrollzentrale auf.
- Ein Rechner trägt die Rolle **Kontrollzentrale**: Go-Backend + Postgres + RustDesk-Server, alles als Container.
- Alle anderen Rollen sind **vom Hersteller signierte, versionierte Templates** (Docker Compose + Hooks + Healthchecks). Der Agent führt nur diese Templates und eine feste Liste typisierter Befehle aus. Jeder Befehl ist zusätzlich von der Kontrollzentrale signiert.
- Die **Kontroll-App** ist eine React/TypeScript-Oberfläche. Derselbe Code läuft als Electron-App und als Web-UI, die die Kontrollzentrale ausliefert.
- **„Einzelserver“ ist technisch ein Cluster mit einem Node**: Kontrollzentrale und Workload-Rollen laufen auf derselben Maschine. Weitere Nodes lassen sich später ohne Umbau hinzufügen.

---

## 1. Klartext: was nicht so geht wie beschrieben

Format: **Befund** → **Vorschlag**. Belege stehen in Abschnitt 18.

### 1.1 Docker unter Windows ist kein Serverdienst
**Befund:** Alle v1-Rollen brauchen Linux-Container. Unter Windows geht das nur über WSL2 oder Hyper-V.
- Docker Desktop startet nur mit angemeldetem Benutzer. Docker selbst sagt, dass Docker Desktop nicht für den Betrieb ohne Login gedacht ist. Außerdem ist Docker Desktop für Firmen ab 250 Mitarbeitern oder 10 Mio. USD Umsatz kostenpflichtig, und diese Bedingungen gelten für den Endkunden.
- Docker Engine läuft unter Windows zwar als Dienst, kann dort aber nur Windows-Container. Minecraft, Postgres, Nextcloud und Ollama gibt es nicht als Windows-Container.

**Vorschlag:**
- **Weg B (Ziel):** Docker Engine (Moby, Apache-2.0) in einer eigenen WSL2-Distro, die der Agent-Dienst beim Booten startet. Kein Login nötig, keine Docker-Desktop-Lizenz. Ob WSL zuverlässig aus einem Dienst ohne angemeldeten Benutzer läuft, ist **nicht gesichert**. Das klärt ein Spike in Meilenstein a.
- **Fallback A:** Docker Desktop, dazu automatische Anmeldung eines eigenen lokalen Benutzers mit sofortiger Bildschirmsperre. Das ist ein Kompromiss und wird in der UI auch so benannt.
- In jedem Fall empfiehlt der Wizard für Server-Rollen Linux.
- Windows braucht für A und B eingeschaltetes VT-x/AMD-V im BIOS. Docker unter Linux braucht **keine** Virtualisierung. Der Virtualisierungs-Check ist deshalb nur unter Windows Pflicht.

**Stand Spike:** WSL verweigert LocalSystem (Quellcode). Weg B läuft deshalb über ein eigenes Konto ohne Admin-Rechte und eine Aufgabe beim Systemstart. Details und Praxistest: [ADR-0002](docs/adr/0002-windows-container-runtime.md), Testkit `tools/spikes/windows-docker-wsl/`.

### 1.2 MinIO ist als Community-Edition tot
**Befund:** Seit Oktober 2025 gibt es keine offiziellen Images mehr. Im Dezember 2025 ging das Projekt in den Wartungsmodus, im Februar 2026 wurde das Repository archiviert.
**Vorschlag:**
- **Nextcloud** als Benutzer-Cloud: Dateien, Web-Oberfläche, Sync-Clients.
- **Garage** (AGPL-3.0, gebaut für kleine selbstgehostete Cluster auf gemischter Hardware) als S3-Speicher mit 2- oder 3-facher Replikation über mehrere Nodes.

### 1.3 exo passt nicht, Ollama kann nicht verteilen
**Befund:**
- exo konzentriert sich auf Apple Silicon/MLX. Windows wird nicht offiziell unterstützt, der Linux-Status ist unklar.
- Ollama kann ein Modell nicht auf mehrere Rechner aufteilen.
- llama.cpp RPC kann das, ist aber **unauthentifiziert**. 2026 gab es dazu kritische CVEs (CVE-2026-34159, CVSS 9.8, gefixt in b8492, und CVE-2026-39909).

**Vorschlag:**
- Single-Node: **Ollama**.
- Multi-Node: **llama.cpp** (`llama-server` als Koordinator, `rpc-server` auf den Workern).
- `rpc-server` lauscht nur auf `127.0.0.1`. Der Verkehr zwischen Nodes läuft durch einen mTLS-Tunnel des Agents. Die Mindestversion pinnt das Template.

**Ehrlich:** Verteiltes Ausführen auf alten Laptops im LAN ist fast immer **langsamer** als ein einzelner Node. Der Nutzen: Modelle laufen, die in keinen einzelnen Arbeitsspeicher passen. Die UI sagt „gemeinsam ausgeführt (größeres Modell, nicht schneller)“. WLAN ist dafür ungeeignet.

### 1.4 Minecraft lässt sich nicht lastverteilen
**Befund:** Eine Welt läuft auf genau einem Rechner. Auch Folia verteilt nur auf die Kerne einer Maschine.
**Vorschlag:** Mehrere Gameserver-Nodes bedeuten mehrere unabhängige Server. Optional lassen sie sich hinter einem Velocity-Proxy zu einem Netzwerk verbinden, dann wechseln Spieler per `/server`. → Entscheidung E6.

### 1.5 Datenbank: Replikation ja, automatisches Failover nein (v1)
**Befund:** Ein automatisches Failover ohne Split-Brain braucht mindestens 3 Nodes und zusätzliche Software (Patroni + etcd, Redis Sentinel). Schreibzugriffe lassen sich nicht auf mehrere Nodes verteilen.
**Vorschlag:** v1 hat Primary + Replicas. Umgeschaltet wird manuell per Knopf („Replica zum Primary machen“). Die UI bezeichnet das als **Redundanz, nicht Lastverteilung**.

### 1.6 Remote-Desktop: Lizenz, Wayland, Passwortübergabe
- **Lizenz:** RustDesk-Client und -Server stehen unter AGPL-3.0. Unverändert als **separate Programme** betreiben und ausliefern ist mit Closed Source vereinbar, wenn Lizenztext und Quellcode-Hinweis beiliegen. RustDesk-Code in die App einbauen oder linken geht **nicht**, sonst fällt die App unter die AGPL. Die Kontroll-App startet deshalb den installierten RustDesk-Client als externen Prozess.
- **Wayland:** Die stabile RustDesk-Version (1.4.6) kann unter Wayland nicht unbeaufsichtigt arbeiten. Nur eine Vorschauversion kann das, und nur für Debian/Ubuntu x86_64. In v1 nutzen Linux-Nodes mit Remote-Desktop deshalb eine **X11-Sitzung**:
  - Bei der geführten Installation kommen XFCE und LightDM aufs System.
  - Bei Ubuntu Desktop stellt der Agent GDM auf X11 um, nach Zustimmung und mit Neustart.
- **Linux ohne Desktop:** Dort gibt es nichts, was Remote-Desktop zeigen könnte. Die UI bietet an, einen Desktop nachzuinstallieren.
- **Nicht verifiziert:** Ob `rustdesk --connect <id>` ein Passwort automatisch übernimmt, ist offiziell nicht dokumentiert. Falls nicht, zeigt die App das Passwort mit Kopier-Knopf. Das klärt ein Spike in Meilenstein f.
- **Reine Web-UI:** Remote-Desktop funktioniert nur, wenn auf dem PC der RustDesk-Client installiert ist.

### 1.7 BIOS-Anleitung „für genau dieses Modell“
**Befund:** Hersteller, Modell und BIOS-Version lassen sich auslesen (SMBIOS/DMI). Eine öffentliche Datenbank mit BIOS-Menüpfaden pro Modell gibt es aber nicht.
**Vorschlag:** Eine eigene, kuratierte Datenbank in zwei Stufen:
1. **Herstellerebene** für die etwa 10 häufigsten Hersteller: Setup-Taste, Boot-Menü-Taste, typische Menüpfade.
2. **Modellebene**, wo bekannt. Diese Stufe wächst mit der Zeit.

Dazu ein Knopf **„Jetzt ins BIOS neu starten“**: unter Windows `shutdown /r /fw /t 0`, unter Linux `systemctl reboot --firmware-setup`, beides nur bei UEFI. Danach läuft der Re-Check automatisch.

### 1.8 Wake-on-LAN
**Befund:** Bei vielen Laptops funktioniert WoL nur über LAN-Kabel, oft nur aus dem Standby statt aus „Aus“ und oft nur am Netzteil. Über WLAN praktisch nie. Der Check meldet das ehrlich.
**Offen:** Die Anforderung prüft WoL, nutzt es aber nirgends. → Entscheidung E11 („Aufwecken“-Knopf ja/nein).

### 1.9 Temperaturen unter Windows
**Befund:** Die CPU-Temperatur ist unter Windows ohne Kernel-Treiber oft nicht lesbar. Der übliche Treiber, WinRing0, gilt als verwundbar und wird seit 2025 von Microsoft Defender als HackTool erkannt.
**Vorschlag:** Kein Kernel-Treiber. Werte kommen aus der ACPI-Thermalzone und für NVIDIA-GPUs aus NVML, jeweils best effort. Fehlt ein Wert, zeigt die UI „nicht verfügbar“ statt einer falschen Zahl. Unter Linux liefert hwmon zuverlässige Werte.

### 1.10 QR-Code beim Node-Hinzufügen
**Befund:** Ein QR-Code auf dem Bildschirm des Haupt-PCs müsste vom alten Laptop gescannt werden. Dem fehlt meist eine passende Kamera oder Software.
**Vorschlag:** Ein **Join-Code** (20 Zeichen, 100 Bit) plus automatische Suche der Kontrollzentrale im LAN (mDNS). → Entscheidung E9.

### 1.11 Offene Registrierung
**Befund:** Wenn sich auf einem privaten Cluster jeder registrieren kann, ist das ein Sicherheitsloch. „Passwort vergessen“ per E-Mail bräuchte einen SMTP-Server, und der steht nicht in der Anforderung.
**Vorschlag:**
- Der erste Account ist Admin und wird durch einen **Setup-Code** geschützt, den die Kontrollzentrale bei der Installation anzeigt.
- Weitere Accounts gibt es nur per **Einladung durch einen Admin**.
- Passwörter setzt ein Admin zurück. Den Admin selbst setzt man per CLI auf dem Rechner der Kontrollzentrale zurück.

→ Entscheidung E7.

### 1.12 TLS ohne Domain
**Befund:** Ohne eigene Domain gibt es kein öffentlich vertrauenswürdiges Zertifikat. Die Kontrollzentrale nutzt deshalb eine eigene Cluster-CA.
- Die Electron-App pinnt diese CA und zeigt **keine Warnungen**.
- Ein normaler Browser warnt, bis die CA importiert ist.

**Vorschlag:** Der Wizard bietet die CA zum Download an, mit Anleitung pro Betriebssystem. Ganz ohne Warnung ginge es nur mit eigener Domain oder einem DNS-Dienst des Herstellers, also zusätzlicher Infrastruktur, die nicht in der Anforderung steht. → Entscheidung E15.

### 1.13 Lizenzdurchsetzung hat Grenzen
**Befund:** In selbstgehosteter Software lässt sich eine Lizenzprüfung nie vollständig durchsetzen, denn wer das Binary patcht, umgeht sie.
- Offline signierte Keys (Ed25519) sind fälschungssicher, lassen sich aber auf beliebig vielen Clustern verwenden.
- Eine Bindung an einen Cluster braucht Online-Aktivierung. Dafür wäre ein **Lizenzserver des Herstellers** nötig, also eine neue Komponente, die nicht im Monorepo-Plan steht.

**Vorschlag:**
- v1 nutzt offline signierte Keys.
- Das Backend wird in **Go** statt Node.js geschrieben. Ein kompiliertes Binary lässt sich deutlich schwerer lesen und patchen als ein JS-Bundle.

→ Entscheidungen E1 und E10.

### 1.14 Windows 10 und 8, 32-Bit
- **Windows 10:** Der reguläre Support endete am 14.10.2025. Die Consumer-ESU wurde bis **12.10.2027** verlängert. Vorschlag: Windows 10 22H2 x64 wird mit Warnhinweis bis zum ESU-Ende unterstützt, danach behandelt wie Windows 8. → E3.
- **Windows 8:** Aktuelle Go-Versionen laufen erst ab Windows 10, der Agent startet dort also gar nicht. Die geführte Linux-Installation muss deshalb **vom Haupt-PC aus** laufen (USB-Stick-Ersteller in der Kontroll-App), nicht vom alten Rechner.
- **32-Bit-x86:** Docker unterstützt es nicht. Debian 13 hat keinen i386-Installer mehr, Ubuntu seit Langem nicht. **Mindestanforderung ist x86-64.** ARM, etwa ein Raspberry Pi, steht nicht in der Anforderung.

### 1.15 Laptops als Server
**Befund:** Klappt man den Deckel zu, geht ein Laptop in den Standby. Energiesparoptionen legen Dienste schlafen. Ohne Anpassung ist der Server dann einfach weg.
Dazu ein Sicherheitsthema: Alte Li-Ion-Akkus, die dauerhaft auf 100 % geladen und warm sind, können sich aufblähen.
**Vorschlag:**
- Der Agent setzt nach Zustimmung im Wizard die Energieoptionen: Deckel zu ohne Folgen, kein Standby und kein Ruhezustand am Netzteil.
- Der Hardware-Check warnt beim Akku. Er empfiehlt eine Ladegrenze, wo Hersteller oder Linux eine anbieten, sonst den Akku zu entfernen.

Das steht nicht in der Anforderung, ist aber Voraussetzung für „idiotensicher“. → E12.

### 1.16 Code-Signing
**Befund:** Ein Windows-Installer ohne Authenticode-Zertifikat löst die SmartScreen-Warnung „Unbekannter Herausgeber“ aus. Virenscanner melden ungesignierte Go-Binaries außerdem häufig fälschlich.
**Vorschlag:** Ein Code-Signing-Zertifikat ist für das Produkt Pflicht. Es verursacht laufende Kosten. → E18.

### 1.17 Kontrollzentrale ist Single Point of Control
**Befund:**
- Fällt die Kontrollzentrale aus, laufen die Dienste auf den anderen Nodes weiter, lassen sich aber nicht mehr steuern. Hochverfügbarkeit für die Kontrollzentrale ist in v1 nicht vorgesehen.
- Ohne Backup heißt ein Plattentod: Cluster neu aufsetzen und alle Nodes neu koppeln, weil CA und Schlüssel weg sind. Backups stehen nicht in der Anforderung.

→ E16.

### 1.18 Erreichbarkeit aus dem Internet
**Befund:** Damit Website oder Minecraft aus dem Internet erreichbar sind, braucht es eine Portfreigabe im Router. Bei CGNAT oder DS-Lite, häufig bei Kabel- und Glasfaseranschlüssen, ist das unmöglich. Das steht nicht in der Anforderung.
**Vorschlag:** v1 ist LAN-only, mit einer Anleitung zur Portfreigabe. → E14.

### 1.19 SmartCluster ist kein Proxmox
SmartCluster nutzt Container statt VMs. Es gibt keine Live-Migration und kein automatisches Umziehen von Diensten auf einen anderen Node, wenn einer ausfällt.

---

## 2. Systemüberblick

```
                         Haupt-PC
        ┌────────────────────────────────────────┐
        │ Kontroll-App (Electron) oder Browser   │
        │ React-UI · RustDesk-Client (extern)    │
        └──────┬───────────────────────┬─────────┘
   HTTPS + WSS │ :8443                 │ RustDesk :21115–21119
               ▼                       ▼
   ┌───────────────────────────────────────────────────┐
   │ Node mit Rolle „Kontrollzentrale“                 │
   │  Container: control (Go) · postgres ·             │
   │             rustdesk-hbbs · rustdesk-hbbr         │
   │  Agent (Dienst) · Docker · ggf. weitere Rollen    │
   └──────────▲──────────────────────▲─────────────────┘
   mTLS-WSS   │ :8444 (ausgehend)    │ :8444
   ┌──────────┴────────┐  ┌──────────┴────────┐
   │ Node B            │  │ Node C            │  …
   │ Agent (Dienst)    │◄─┤ Agent (Dienst)    │  mTLS-Tunnel :8446
   │ Docker + Rollen   │  │ Docker + Rollen   │  (nur AI-Multi-Node)
   │ RustDesk-Client   │  │ RustDesk-Client   │
   └───────────────────┘  └───────────────────┘
```

**Grundprinzipien**
1. **Agents öffnen keine eingehenden Ports.** Einzige Ausnahme ist der AI-Worker-Tunnel :8446, nur mit mTLS.
2. **Code auf einem Node läuft nur aus zwei Quellen:** aus einem vom Hersteller signierten Rollen-Template oder aus einem fest im Agent einprogrammierten Befehlstyp. Beliebige Shell-Kommandos von außen gibt es nicht.
3. **Eine Quelle der Wahrheit für Schnittstellen:** OpenAPI 3.1 und JSON-Schema liegen in `/docs`. Go- und TS-Typen werden daraus generiert bzw. dagegen getestet.
4. **Idempotenz:** Jede Rollenanwendung lässt sich gefahrlos wiederholen.
5. **Ehrliche UI:** Jede Multi-Node-Rolle zeigt an, ob sie Lastverteilung, Redundanz, Modell-Aufteilung oder nur unabhängige Instanzen bietet.

---

## 3. Komponenten

### 3.1 Agent (`/agent`, Go)

**Plattformen v1**
- Windows 11 x64
- Windows 10 22H2 x64 (mit Warnung, siehe 1.14)
- Debian 12/13 amd64
- Ubuntu 22.04, 24.04 und 26.04 LTS amd64

**Laufzeit:** Ein Binary `smartcluster-agent`. Es läuft unter Windows als Dienst mit LocalSystem, unter Linux als systemd-Dienst mit root. Der Installer fordert die Rechte an (UAC bzw. sudo).

| Modul | Aufgabe |
|---|---|
| `service` | Installation, Start und Stopp als Dienst (kardianos/service) |
| `enroll` | Kopplung per Join-Code, Schlüsselpaar, CSR, Zertifikatserneuerung |
| `conn` | mTLS-WebSocket, Reconnect mit Backoff 1 s → 60 s plus Jitter, Heartbeat |
| `verify` | Prüft signierte Befehle (Cluster-Key) und Rollen-Bundles (Hersteller-Key) |
| `stats` | CPU, RAM, Disk, Netz, Uptime, OS (gopsutil v4). GPU: NVML für NVIDIA, sonst Basisdaten. Temperaturen: hwmon bzw. ACPI best effort. Akku |
| `hwcheck` | Virtualisierung, WoL, Firmware (UEFI/Legacy, Secure Boot), Hersteller/Modell/BIOS (SMBIOS), LAN oder WLAN mit Linkspeed, Disk-Typ, Energieoptionen, OS-Support, Uhrzeit |
| `docker` | Erkennen und Installieren. Linux: offizielles Docker-APT-Repo mit gepinntem GPG-Fingerprint. Windows: siehe 1.1. Zugriff über die Engine API |
| `engine` | Nur Windows: Konto `smartcluster-engine`, Startaufgabe und `engine-keeper` (WSL2-Distro mit dockerd, Named Pipe für Hooks), siehe ADR-0002 |
| `roles` | Bundle prüfen und entpacken, `.env` erzeugen, `docker compose` mit festen Argumenten aufrufen, Hooks und Healthchecks ausführen |
| `logs` | Container-Logs über die Docker API und das eigene Agent-Log, mit Rate-Limit |
| `power` | Energieoptionen setzen, nur nach Freigabe (E12) |
| `tunnel` | mTLS-TCP-Tunnel zwischen Nodes, nur für AI-Multi-Node |
| `rustdesk` | RustDesk-Client installieren und konfigurieren (Meilenstein f) |
| `update` | Selbst-Update: Binary herunterladen, Hersteller-Signatur prüfen, atomar tauschen, neu starten. Startet die neue Version nicht, wird zurückgerollt |

**Lokale Daten**
- Linux: `/etc/smartcluster/agent.yaml` und `/var/lib/smartcluster/`. Schlüssel liegen mit Rechten 0600, Rollendaten unter `roles/<assignment-id>/`.
- Windows: `C:\ProgramData\SmartCluster\`, mit ACL nur für SYSTEM und Administratoren.

**Zustände:** `unpaired → enrolling → connected ⇄ reconnecting`. Auf dem ersten Node gibt es zusätzlich den Zustand `bootstrap`.

**Lokale CLI** (nur als root bzw. Admin):
```
smartcluster-agent init                          # neuer Cluster, dieser Rechner = Kontrollzentrale
smartcluster-agent join <CODE> [--control <adresse>]
smartcluster-agent status                        # Verbindung, Node-ID, CA-Fingerprint
smartcluster-agent service install|uninstall|start|stop
```

### 3.2 Kontrollzentrale (`/control`)

**Sprache: Go** (Empfehlung, Entscheidung E1). Gründe:
- Gemeinsamer Code mit dem Agent: Protokolltypen, Signaturprüfung, Template-Parser.
- Ein Binary mit wenig RAM-Bedarf, was auf alten Laptops zählt.
- Schwerer zu patchen als JavaScript (siehe 1.13).

Die React-UI bleibt TypeScript. Die API-Typen werden aus OpenAPI generiert.

**Container der Rolle `control`**

| Container | Inhalt | Ports |
|---|---|---|
| `control` | Go-Binary: REST, WebSockets, eingebettete Web-UI, Job-Engine, PKI | 8443 (UI/API), 8444 (Agents) |
| `postgres` | Postgres 18, nur im internen Docker-Netz erreichbar | – |
| `rustdesk-hbbs` | RustDesk-ID- und Rendezvous-Server | 21115, 21116 tcp+udp, 21118 |
| `rustdesk-hbbr` | RustDesk-Relay | 21117, 21119 |

RAM-Bedarf etwa 400–600 MB. Der Node für die Kontrollzentrale braucht mindestens 2 GB RAM, empfohlen sind 4 GB.

**Interne Pakete:** `api`, `auth`, `live` (App-WS), `agentgw` (Agent-WS + Enrollment), `registry`, `stats`, `roles`, `engine`, `jobs`, `alerts`, `audit`, `license`, `secrets`, `pki`, `rustdesk`, `biosguide`, `setup`.

**Bibliotheken (Vorschlag):** `net/http` (ServeMux), `pgx` v5 + `sqlc`, `goose` (Migrationen), `coder/websocket`, `x/crypto/argon2`, `golang-jwt/jwt` v5 (EdDSA), `log/slog`, `santhosh-tekuri/jsonschema`, `grandcat/zeroconf` (mDNS).

**Job-Engine**
- Ablauf: Änderung an einem Assignment → **Plan** (geordnete Schritte pro Node) → **Job** mit Fortschritt → pro Schritt ein **signierter Command**.
- Die Reihenfolge kommt aus der Template-Topologie, zum Beispiel DB-Primary vor den Replicas und AI-Worker vor dem Koordinator.
- Es gibt kein transaktionales Rollback, denn Docker-Seiteneffekte lassen sich nicht sauber zurücknehmen. Bei einem Fehler wird der Job `failed`, der Zustand bleibt sichtbar, und die UI bietet „Erneut versuchen“ (Apply ist idempotent) oder „Rolle entfernen“.

**Warnungen** (feste Schwellen in v1)
- Node offline seit mehr als 60 s
- Disk über 90 %
- RAM über 95 % für 5 min
- CPU-Temperatur über 90 °C
- Healthcheck rot
- Node läuft auf Akku (deutet auf Stromausfall)
- Zertifikat oder Lizenz läuft in weniger als 14 Tagen ab
- Uhrzeit weicht mehr als 5 min ab

### 3.3 Kontroll-App (`/app`, Electron + React/TypeScript)

- **Stack:** React 19, TypeScript strict, Vite, react-router, TanStack Query (REST), eigener WS-Client, i18next (de/en), uPlot (Zeitreihen), Radix-UI-Primitives (barrierefreie Dialoge und Menüs), Design-Tokens als CSS-Variablen für Light und Dark Mode.
- **Zwei Build-Ziele aus einem Code:**
  - **Web-UI:** von der Kontrollzentrale unter `https://<host>:8443/` ausgeliefert, per `go:embed` eingebettet.
  - **Electron:** Renderer gebündelt, damit die App ohne Kontrollzentrale startet. Versionsabgleich über `GET /api/v1/version`.
- **Nur in Electron (Main-Prozess):**
  - mDNS-Suche nach `_smartcluster._tcp`
  - CA-Pinning über `setCertificateVerifyProc`
  - Refresh-Token in `safeStorage`
  - Start des RustDesk-Clients
  - USB-Stick-Ersteller (Meilenstein g)
- **Electron-Härtung:**
  - `contextIsolation` und `sandbox` an, `nodeIntegration` aus
  - strikte CSP
  - Der Preload gibt nur `window.smartcluster.{discover, trustCluster, secureStore, launchRemoteDesktop, usb}` frei.
- **Plattformen Electron v1:** Windows x64 und Linux x64 (macOS → E20).

**Ansichten**

| Ansicht | Inhalt |
|---|---|
| Onboarding-Wizard | siehe 4.1 |
| Dashboard | Kachel pro Node: Status, Rollen, CPU/RAM/Disk-Sparkline, Temperatur, Warnungen. Dazu Cluster-Summen |
| Node-Detail | Graphen (1 h / 24 h / 7 d / 90 d), Container, Live-Logs, Hardware-Check mit Anleitung, Aktionen (Rolle ändern, Neustart, Neustart ins BIOS, Update, Entfernen), Remote-Desktop-Knopf |
| Cluster-Ansicht | Graph aus Nodes und Rollen-Instanzen. Verbindungen kommen aus der Topologie (LB → Backends, Primary → Replicas, Koordinator → Worker). Auslastung farbig |
| Rollen | Katalog, laufende Instanzen, Assistent „Rolle zuweisen“ mit einem Formular, das aus dem Parameter-Schema generiert wird |
| Jobs | Fortschritt Schritt für Schritt, Fehler mit Erklärung |
| Einstellungen | Benutzer (Admin), Lizenz, Sprache, Theme, CA-Download |
| Audit-Log | nur Admin, mit Filtern |

---

## 4. Abläufe

### 4.1 Erster Rechner (neuer Cluster) und Onboarding

```
Laptop 1 (Installer, Admin-Rechte)               Haupt-PC (Kontroll-App)
──────────────────────────────────               ──────────────────────────
1. „Neuen SmartCluster starten“
2. Vorabcheck: x86-64, RAM ≥ 2 GB, Disk, OS
3. Docker prüfen bzw. installieren
4. Rolle „control“ aus dem eingebetteten,
   signierten Bundle anwenden
5. control erzeugt CA, Schlüssel und Master-Key;
   der Agent koppelt sich lokal (Bootstrap-Token)
6. Anzeige: Adresse + Setup-Code (20 Zeichen)
                                                  7. Kontrollzentrale per mDNS finden
                                                     oder Adresse eingeben
                                                  8. Setup-Code eingeben: authentifiziert die
                                                     Kontrollzentrale (CA wird gepinnt) und
                                                     erlaubt den ersten Account
                                                  9. Account anlegen (Admin)
                                                 10. Lizenz eingeben (ab Meilenstein g)
                                                 11. „Einzelserver oder Cluster?“
                                                 12. Cluster: weitere Nodes hinzufügen (4.2)
                                                 13. Hardware-Check pro Node, Anleitung, Re-Check
                                                 14. Rolle(n) wählen → Einrichtung mit Fortschritt
```

**Einzelserver:** Schritt 12 entfällt, und die gewählte Rolle läuft zusätzlich auf Laptop 1.
**Cluster:** Laptop 1 kann reine Kontrollzentrale bleiben oder zusätzlich Rollen tragen.

### 4.2 Weiteren Node hinzufügen
1. In der App unter „Node hinzufügen“ einen Join-Code erzeugen. Er hat 20 Zeichen Crockford-Base32 (100 Bit), gilt 30 min und nur einmal. Die App zeigt ihn zusammen mit der Adresse der Kontrollzentrale an.
2. Auf dem neuen Rechner den Installer starten und „Bestehendem SmartCluster beitreten“ wählen. Den Code eingeben. Die Adresse wird per mDNS gefunden, sonst trägt man sie ein.
3. Enrollment (4.3). Der Node erscheint live in der App, und der Wizard läuft mit dem Hardware-Check weiter.

Ohne Bildschirm (Linux-Server): `sudo smartcluster-agent join <CODE>`. Bei der geführten Linux-Installation steckt der Code bereits im USB-Stick.

### 4.3 Enrollment mit Kanalbindung (gegen Man-in-the-Middle)
Ziel: Agent und Kontrollzentrale weisen sich gegenseitig über den gemeinsamen Join-Code aus, ohne dass jemand Fingerprints von Hand vergleichen muss.

1. Der Agent erzeugt ein ECDSA-P-256-Schlüsselpaar und einen CSR.
2. Der Agent baut TLS zu `:8444` auf, ohne das Zertifikat zu prüfen, merkt sich aber die präsentierte Zertifikatskette `chain`.
3. `POST /agent/v1/enroll` mit `{csr, hw_summary, nonce_a, proof_a}`. Dabei gilt `proof_a = HMAC-SHA256(K, "agent" ‖ fp(chain) ‖ sha256(csr) ‖ nonce_a)` mit `K = HKDF(join_code)`.
4. Die Kontrollzentrale berechnet `proof_a` mit dem Fingerprint ihres **eigenen** Zertifikats. Sitzt ein Angreifer dazwischen, präsentiert er ein anderes Zertifikat, der Fingerprint weicht ab und die Prüfung schlägt fehl.
5. Antwort: `{client_cert, ca_cert, cmd_pubkeys, node_id, proof_s}` mit `proof_s = HMAC(K, "server" ‖ fp(chain) ‖ sha256(client_cert) ‖ nonce_a)`.
6. Der Agent prüft `proof_s` und speichert danach CA und Command-Keys als Vertrauensanker.

Der Join-Code wird verbraucht. Das Client-Zertifikat gilt 1 Jahr und wird ab 30 Tagen vor Ablauf über die bestehende Verbindung erneuert.
Der **Setup-Code** für die App (4.1, Schritt 8) nutzt dasselbe Verfahren, nur ohne CSR. Danach pinnt die App die CA.
Weitere Geräte und Browser: Die App zeigt den CA-Fingerprint als 6 Wortgruppen an. Er lässt sich mit `smartcluster-agent status` oder einem bereits angemeldeten Gerät vergleichen.

### 4.4 Hardware-Check und BIOS-Anleitung

| Prüfung | Windows | Linux | Bedeutung |
|---|---|---|---|
| CPU x86-64 | Laufzeit | Laufzeit | Pflicht |
| Virtualisierung | WMI `Win32_Processor.VirtualizationFirmwareEnabled`, `Win32_ComputerSystem.HypervisorPresent` | cpuinfo `vmx`/`svm`, `/dev/kvm` | Pflicht nur unter Windows (1.1) |
| Wake-on-LAN | NIC-Power-Management (WMI/NetAdapter) | ethtool-ioctl | Info (E11) |
| Firmware | UEFI/Legacy, Secure Boot | `/sys/firmware/efi`, efivars | Info: BIOS-Neustart, USB-Boot |
| RAM, Disk, SSD/HDD | WMI | sysfs | gegen Rollen-Anforderungen |
| Netzwerk | Adaptertyp, Linkspeed | sysfs | Warnung bei WLAN für Server-Rollen |
| OS-Version | Build, EOL | os-release | Warnung bzw. Fehler |
| Docker | vorhanden, funktioniert | vorhanden, funktioniert | Pflicht für Workload-Rollen |
| Energie | powercfg | logind, sleep.target | E12 |
| Akku | WMI-Batterie | `/sys/class/power_supply` | Warnung (1.15) |
| Uhrzeit | Abweichung zur Kontrollzentrale | dito | nötig für signierte Befehle |

Ergebnis je Prüfung: `ok | warn | fail | n/a`, dazu ein Text-Key (de/en) und gegebenenfalls ein Anleitungsverweis.

**BIOS-Guide-Datenbank** (`/control/biosguides/*.yaml`, Format):
```yaml
vendor: LENOVO
match: { sys_vendor: "^LENOVO$" }
keys:  { setup: ["F1"], bootMenu: ["F12"] }
settings:
  virtualization:
    de: "Security → Virtualization → Intel Virtualization Technology → Enabled"
    en: "Security → Virtualization → Intel Virtualization Technology → Enabled"
  wakeOnLan:
    de: "Config → Network → Wake On LAN → AC Only"
models:
  - match: { product_version: "^ThinkPad T4[23]0" }
    keys: { setup: ["F1"] }
```
**Ablauf:** Check schlägt fehl → Anleitung (Taste und Pfad) → „Jetzt ins BIOS neu starten“ → der Nutzer ändert die Einstellung → der Rechner bootet und der Agent verbindet sich neu → automatischer Re-Check → der Wizard geht weiter.

### 4.5 Rolle zuweisen
1. Der Nutzer wählt Rolle, Version und Node(s) und die Member-Art, etwa Primary oder Replica.
2. Die App ruft `…/check` auf und sieht pro Node, ob die Anforderungen erfüllt sind. Ungeeignete Nodes sind ausgegraut, mit Begründung.
3. Das Parameter-Formular wird aus dem Schema generiert.
4. `POST /assignments` → Plan → Job.
5. Pro Node: `docker.ensure` → `role.apply` mit Fortschritt (Image-Pull, Hooks, Healthchecks).
6. Healthchecks grün → Assignment `running`, und die UI zeigt die Zugangsdaten an.

---

## 5. Sicherheitsmodell

### 5.1 Schlüssel und Vertrauensanker

| Schlüssel | Typ | Erzeugt | Liegt | Zweck |
|---|---|---|---|---|
| Hersteller-Release-Key | Ed25519 | einmalig, offline | geschützte CI-Release-Umgebung | signiert Agent- und Control-Releases sowie Update-Manifeste |
| Hersteller-Rollen-Key | Ed25519 | einmalig, offline | geschützte CI-Release-Umgebung | signiert Rollen-Bundles und den Katalog |
| Hersteller-Lizenz-Key | Ed25519 | einmalig, offline | nur beim Hersteller (`licensegen`) | signiert Lizenzschlüssel |
| Cluster-CA | ECDSA P-256 | Bootstrap | DB, verschlüsselt | Server-, Agent- und Replikationszertifikate |
| Command-Key | Ed25519 | Bootstrap | DB, verschlüsselt | signiert Befehle an Agents |
| JWT-Key | Ed25519 | Bootstrap | DB, verschlüsselt | signiert Access-Tokens |
| Master-Key | 256 Bit | Bootstrap | Datei mit 0600 im Datenverzeichnis der Rolle `control` | verschlüsselt Secrets und private Schlüssel in der DB |
| Agent-Schlüssel | ECDSA P-256 | auf dem Node | lokal 0600 bzw. ACL | mTLS-Client-Identität |

Die öffentlichen Herstellerschlüssel sind in die Binaries einkompiliert. Pro Zweck sind mehrere Key-IDs erlaubt, damit sich Schlüssel rotieren lassen.
**Ehrlich:** Der Master-Key liegt auf demselben Rechner wie die DB. Er schützt gegen DB-Dumps und Backups, aber nicht gegen jemanden mit root-Rechten auf der Kontrollzentrale.

### 5.2 Transport

| Verbindung | Schutz |
|---|---|
| App ↔ Kontrollzentrale | HTTPS/WSS :8443, TLS 1.3, Zertifikat der Cluster-CA, Electron pinnt die CA |
| Agent ↔ Kontrollzentrale | WSS :8444, TLS 1.3, **mTLS** (Client-Zertifikat Pflicht), Sperrliste für entfernte Nodes |
| Agent ↔ Agent (AI-Tunnel) | TLS 1.3 mTLS :8446, nur zwischen Nodes desselben Assignments |
| DB-Replikation | native TLS von Postgres, MySQL und Valkey, mit Zertifikaten der Cluster-CA |
| Garage intern | Garage-eigene verschlüsselte RPC mit Shared Secret (Details in Meilenstein d verifizieren) |
| RustDesk | RustDesk-eigene Verschlüsselung mit dem Schlüsselpaar des eigenen hbbs |
| Endnutzer → Dienste | Protokoll des jeweiligen Dienstes: Website über Caddy-HTTPS, Minecraft über das eigene Protokoll |

„Alle Verbindungen TLS“ gilt damit für jede **SmartCluster-interne** Verbindung. Endnutzer-Protokolle wie Minecraft bestimmt der jeweilige Dienst.

### 5.3 Signierte Befehle
```json
{
  "v": 1,
  "kid": "cmd-2026-10-14",
  "cmd_id": "01JA2Z6Q3B8V0X7K9M4N5P6R7S",
  "node_id": "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e",
  "type": "role.apply",
  "issued_at": "2026-10-14T12:00:00Z",
  "expires_at": "2026-10-14T12:05:00Z",
  "payload": { },
  "sig": "<base64url Ed25519 über RFC-8785-kanonisches JSON ohne sig>"
}
```
**Prüfungen im Agent**, in dieser Reihenfolge:
1. `kid` ist bekannt.
2. Die Signatur ist gültig.
3. `node_id` ist die eigene.
4. Die Zeit liegt im Fenster `issued_at − 30 s … expires_at`. Grundlage ist die korrigierte Uhr; den Offset liefert `hello.ok`.
5. `cmd_id` wurde noch nicht gesehen (Replay-Schutz bis `expires_at` + 1 min).
6. `type` ist bekannt.
7. `payload` entspricht dem Schema dieses Typs.

Schlägt eine Prüfung fehl, antwortet der Agent mit `cmd.reject` und einem Code. Die Kontrollzentrale schreibt einen Audit-Eintrag und eine Warnung.

### 5.4 Signierte Templates, keine beliebige Shell
- Der Agent kennt eine **feste Liste von Befehlstypen** (Abschnitt 9). Code auf dem Node entsteht nur aus Compose-Dateien und Hook-Skripten eines Bundles mit gültiger **Hersteller-Signatur**.
- **Parameter werden nie in Skript- oder YAML-Text eingesetzt.** Sie kommen als validierte Umgebungsvariablen: in `.env` für Compose und als `SC_PARAM_*` für Hooks. Strings sind per Schema-Regex auf ungefährliche Zeichen beschränkt.
- Images sind per Digest gepinnt (`image@sha256:…`).
- CI-Regel für Templates: kein Docker-Socket-Mount, kein `privileged`, kein `network_mode: host`. Ausnahmen müssen im Template begründet und im Review freigegeben werden, etwa GPU-Zugriff über eine Device-Reservierung.
- **Ehrlich:** Eine kompromittierte Kontrollzentrale kann immer noch jedes signierte Template mit gültigen Parametern auf jeden Node anwenden. Die Signatur verhindert beliebigen Code, aber nicht den Missbrauch erlaubter Aktionen. Digest-Pinning garantiert, dass genau das geprüfte Image läuft, nicht dass es fehlerfrei ist.

### 5.5 Benutzer-Authentifizierung
- **Passwörter:** Argon2id mit m = 46 MiB, t = 1, p = 1 (OWASP-Empfehlung), gespeichert als PHC-String. Wegen des RAMs laufen höchstens 2 Hash-Berechnungen gleichzeitig. Mindestlänge 12 Zeichen, Abgleich mit einer lokalen Liste häufiger Passwörter.
- **Access-Token:** JWT mit EdDSA, 15 min gültig. Claims: `sub`, `role`, `sid`, `iat`, `exp`, `jti`.
- **Refresh-Token:**
  - opak, 256 Bit
  - in der DB nur als SHA-256-Hash
  - wird bei jeder Nutzung rotiert
  - wird ein Token wiederverwendet, wird die ganze Familie widerrufen
  - 30 Tage gleitend
- **Speicherung:**
  - Web: Access-Token nur im Arbeitsspeicher, Refresh-Token als `httpOnly`-, `Secure`- und `SameSite=Strict`-Cookie mit Pfad `/api/v1/auth`. Zusätzlich wird der Origin geprüft.
  - Electron: `safeStorage`.
- **Login-Schutz:** Rate-Limit pro IP und pro Konto mit exponentiellem Backoff. Fehlermeldungen verraten nicht, ob ein Konto existiert.

**Rechte** (Vorschlag E8)

| Aktion | User | Admin |
|---|---|---|
| Dashboard, Stats, Logs, Jobs ansehen | ja | ja |
| Container starten/stoppen, Neustart, Update | nein | ja |
| Rollen zuweisen, ändern, entfernen | nein | ja |
| Nodes hinzufügen/entfernen | nein | ja |
| Remote-Desktop | nein | ja |
| Zugangsdaten (Secrets) anzeigen | nein | ja |
| Benutzer, Lizenz, Audit-Log | nein | ja |

### 5.6 Secrets
- Verschlüsselt mit AES-256-GCM. Die Nonce ist zufällig (96 Bit), als AAD dienen `scope` und `name`. Über `key_version` lässt sich der Schlüssel rotieren.
- Erzeugte Rollen-Secrets wie DB-Passwörter gehen nur beim Anwenden an den Agent: im signierten Befehl, innerhalb von mTLS. Dort landen sie in der Rollen-`.env` mit Rechten 0600.
- In der Tabelle `commands` stehen Payloads nur **geschwärzt**.
- Die UI zeigt Secrets nur auf ausdrücklichen Klick. Jede Anzeige erzeugt einen Audit-Eintrag.

### 5.7 Audit-Log
Das Audit-Log ist append-only: Die DB-Rolle der App darf nur `INSERT` und `SELECT`. Erfasst werden:
- jede schreibende API-Aktion
- Logins und Fehlversuche
- Enrollments und Zertifikatserneuerungen
- abgelehnte Befehle
- Remote-Desktop-Sessions
- Secret-Anzeigen
- Lizenzänderungen

---

## 6. Datenmodell (Postgres 18)

Konventionen: UUIDv7 als Primärschlüssel, `timestamptz`, `snake_case`. Migrationen mit `goose` in `/control/migrations`.

```sql
CREATE TYPE user_role         AS ENUM ('admin', 'user');
CREATE TYPE node_status       AS ENUM ('enrolling', 'online', 'offline', 'provisioning', 'error', 'removed');
CREATE TYPE assignment_status AS ENUM ('pending', 'applying', 'running', 'degraded', 'failed', 'removing', 'removed');
CREATE TYPE job_status        AS ENUM ('queued', 'running', 'succeeded', 'failed', 'cancelled');

-- Benutzer & Sitzungen ------------------------------------------------------
CREATE TABLE users (
  id             uuid PRIMARY KEY,
  email          citext UNIQUE NOT NULL,
  password_hash  text NOT NULL,                 -- Argon2id, PHC-String
  role           user_role NOT NULL,
  locale         text NOT NULL DEFAULT 'de',
  created_at     timestamptz NOT NULL DEFAULT now(),
  last_login_at  timestamptz,
  disabled_at    timestamptz
);
CREATE TABLE invitations (
  id uuid PRIMARY KEY, email citext NOT NULL, role user_role NOT NULL,
  token_hash bytea UNIQUE NOT NULL, created_by uuid NOT NULL REFERENCES users,
  expires_at timestamptz NOT NULL, used_at timestamptz
);
CREATE TABLE refresh_tokens (
  id uuid PRIMARY KEY, user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
  family_id uuid NOT NULL, token_hash bytea UNIQUE NOT NULL,
  created_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
  revoked_at timestamptz, replaced_by uuid, user_agent text, ip inet
);

-- Cluster, Lizenz, Secrets ------------------------------------------------
CREATE TABLE cluster (                          -- genau eine Zeile
  id uuid PRIMARY KEY, name text NOT NULL,
  mode text CHECK (mode IN ('single', 'cluster')),
  ca_cert bytea NOT NULL, created_at timestamptz NOT NULL,
  setup_completed_at timestamptz
);
CREATE TABLE secrets (
  id uuid PRIMARY KEY, scope text NOT NULL, name text NOT NULL,
  ciphertext bytea NOT NULL, nonce bytea NOT NULL, key_version int NOT NULL,
  created_at timestamptz NOT NULL, rotated_at timestamptz,
  UNIQUE (scope, name)
);
CREATE TABLE licenses (
  id uuid PRIMARY KEY, license_id text UNIQUE NOT NULL, key text NOT NULL,
  payload jsonb NOT NULL, status text NOT NULL,     -- valid | expired | invalid
  activated_at timestamptz NOT NULL, activated_by uuid REFERENCES users,
  expires_at timestamptz, last_checked_at timestamptz
);
CREATE TABLE enrollment_codes (                  -- Setup-, Join- und Bootstrap-Codes
  id uuid PRIMARY KEY, purpose text CHECK (purpose IN ('setup', 'join', 'bootstrap')),
  code_key bytea NOT NULL,                       -- HKDF-abgeleitet, nie der Klartext-Code
  created_by uuid REFERENCES users, created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL, used_at timestamptz, used_by_node uuid
);

-- Nodes ---------------------------------------------------------------------
CREATE TABLE nodes (
  id uuid PRIMARY KEY, name text NOT NULL, hostname text,
  status node_status NOT NULL,
  os jsonb, hardware jsonb, capabilities text[] NOT NULL DEFAULT '{}',  -- z. B. docker, nvidia, desktop
  agent_version text, cert_serial text, cert_not_after timestamptz,
  ip_addresses inet[], mac_addresses macaddr[],
  rustdesk_id text,
  created_at timestamptz NOT NULL, last_seen_at timestamptz, removed_at timestamptz
);
CREATE TABLE node_hwchecks (
  id uuid PRIMARY KEY, node_id uuid NOT NULL REFERENCES nodes,
  ran_at timestamptz NOT NULL, result jsonb NOT NULL   -- {check: {state, key, guide}}
);
CREATE TABLE revoked_certs (serial text PRIMARY KEY, node_id uuid, revoked_at timestamptz NOT NULL);

-- Zeitreihen (Tagespartitionen; raw 48 h, 5-min-Rollup 90 Tage) -----------
CREATE TABLE node_stats_raw (
  node_id uuid NOT NULL, ts timestamptz NOT NULL,
  cpu_pct real, load1 real, mem_used bigint, mem_total bigint, swap_used bigint,
  disk_used_pct_max real, net_rx_bps bigint, net_tx_bps bigint,
  cpu_temp_c real, gpu_util_pct real, on_battery boolean,
  detail jsonb,                                  -- vollständiges Stats-Paket
  PRIMARY KEY (node_id, ts)
) PARTITION BY RANGE (ts);
CREATE TABLE node_stats_5m (
  node_id uuid NOT NULL, bucket timestamptz NOT NULL,
  cpu_pct_avg real, cpu_pct_max real, mem_used_avg bigint, mem_used_max bigint,
  disk_used_pct_max real, net_rx_bps_avg bigint, net_tx_bps_avg bigint,
  cpu_temp_c_max real, gpu_util_pct_avg real,
  PRIMARY KEY (node_id, bucket)
) PARTITION BY RANGE (bucket);

-- Rollen --------------------------------------------------------------------
CREATE TABLE role_templates (
  role_id text NOT NULL, version text NOT NULL,
  manifest jsonb NOT NULL, bundle_sha256 bytea NOT NULL,
  signature bytea NOT NULL, key_id text NOT NULL, imported_at timestamptz NOT NULL,
  PRIMARY KEY (role_id, version)
);
CREATE TABLE role_assignments (
  id uuid PRIMARY KEY, role_id text NOT NULL, role_version text NOT NULL,
  name text NOT NULL, params jsonb NOT NULL, status assignment_status NOT NULL,
  created_by uuid REFERENCES users, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  FOREIGN KEY (role_id, role_version) REFERENCES role_templates
);
CREATE TABLE assignment_members (
  assignment_id uuid NOT NULL REFERENCES role_assignments,
  node_id uuid NOT NULL REFERENCES nodes,
  member text NOT NULL,                          -- z. B. primary, replica, worker
  status assignment_status NOT NULL, health jsonb, updated_at timestamptz NOT NULL,
  PRIMARY KEY (assignment_id, node_id, member)
);
CREATE TABLE node_containers (                   -- Cache, vom Agent gemeldet
  node_id uuid NOT NULL REFERENCES nodes, container_id text NOT NULL,
  assignment_id uuid REFERENCES role_assignments, service text,
  image text, state text, updated_at timestamptz NOT NULL,
  PRIMARY KEY (node_id, container_id)
);

-- Jobs & Befehle -------------------------------------------------------------
CREATE TABLE jobs (
  id uuid PRIMARY KEY, kind text NOT NULL,
  assignment_id uuid REFERENCES role_assignments, node_id uuid REFERENCES nodes,
  status job_status NOT NULL, progress smallint NOT NULL DEFAULT 0,
  steps jsonb NOT NULL, error_code text,
  created_by uuid REFERENCES users, created_at timestamptz NOT NULL,
  started_at timestamptz, finished_at timestamptz
);
CREATE TABLE commands (
  id uuid PRIMARY KEY, job_id uuid REFERENCES jobs, node_id uuid NOT NULL REFERENCES nodes,
  type text NOT NULL, payload_redacted jsonb NOT NULL, status text NOT NULL,
  issued_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
  sent_at timestamptz, acked_at timestamptz, finished_at timestamptz, result jsonb
);

-- Warnungen & Audit ----------------------------------------------------------
CREATE TABLE alerts (
  id uuid PRIMARY KEY, node_id uuid REFERENCES nodes, assignment_id uuid REFERENCES role_assignments,
  severity text NOT NULL, kind text NOT NULL, data jsonb,
  created_at timestamptz NOT NULL, acknowledged_by uuid REFERENCES users,
  acknowledged_at timestamptz, resolved_at timestamptz
);
CREATE TABLE audit_log (
  id bigserial PRIMARY KEY, ts timestamptz NOT NULL DEFAULT now(),
  actor_type text NOT NULL,                      -- user | agent | system
  actor_id uuid, action text NOT NULL, target_type text, target_id text,
  ip inet, details jsonb
);
-- GRANT INSERT, SELECT ON audit_log TO sc_app;  (kein UPDATE/DELETE)
```

**Datenmenge:** 10 Nodes × 1 Sample alle 5 s ergeben 172.800 Zeilen pro Tag in `node_stats_raw`. Das schafft Postgres auch auf einem alten Laptop problemlos. Partitionen entfernt ein Wartungsjob in der Kontrollzentrale.

---

## 7. REST-API

**Basis:** `https://<host>:8443/api/v1`, JSON. Quelle der Wahrheit ist `/docs/api/openapi.yaml`. Daraus werden der Go-Server (oapi-codegen) und der TS-Client (openapi-typescript) generiert.
**Fehlerformat:** `{"error": {"code": "node_offline", "message": "…", "details": {}}}`. Die UI übersetzt anhand von `code`.
**Auth:** `Authorization: Bearer <access>`. Listen sind per Cursor paginiert (`?cursor=…&limit=…`).
**Recht:** 🔓 öffentlich · U = User · A = Admin. **MS:** Meilenstein, in dem der Endpunkt kommt.

| Methode & Pfad | Zweck | Recht | MS |
|---|---|---|---|
| `GET /setup/status` | `{initialized, adminExists, mode, licenseState}` | 🔓 | b |
| `POST /setup/handshake` | Setup-Code-Kanalbindung (4.3): `{nonce, proof}` → `{caCert, proof}` | 🔓 | b |
| `POST /setup/admin` | ersten Admin anlegen (Setup-Code-Beweis nötig) | 🔓 | b |
| `PUT /setup/mode` | `single` oder `cluster` | A | b |
| `POST /auth/login` · `/auth/refresh` · `/auth/logout` | Sitzung | 🔓 / Cookie | b |
| `GET /auth/me` · `PUT /auth/me/password` | eigenes Konto | U | b |
| `POST /auth/invitations/accept` | Einladung annehmen und Passwort setzen | 🔓 + Token | b |
| `GET /users` · `POST /users/invitations` · `PATCH /users/{id}` · `DELETE /users/{id}` | Benutzerverwaltung | A | b |
| `POST /users/{id}/password-reset` | Einmal-Link zum Zurücksetzen | A | b |
| `GET /license` · `PUT /license` | Lizenz anzeigen bzw. aktivieren | A | g |
| `POST /join-tokens` · `GET /join-tokens` · `DELETE /join-tokens/{id}` | Join-Codes | A | a (CLI) / b (UI) |
| `GET /nodes` · `GET /nodes/{id}` | Liste und Detail | U | a |
| `PATCH /nodes/{id}` · `DELETE /nodes/{id}` | umbenennen bzw. entfernen (Zertifikat sperren) | A | b |
| `GET /nodes/{id}/stats?from&to&res=raw\|5m` | Zeitreihe | U | a |
| `POST /nodes/{id}/hwcheck` · `GET /nodes/{id}/hwcheck` | Check starten bzw. letztes Ergebnis | A / U | b |
| `GET /nodes/{id}/bios-guide?setting=` | passende Anleitung | U | b |
| `POST /nodes/{id}/reboot` | `{firmwareSetup: bool}` | A | b |
| `POST /nodes/{id}/power-settings` | Energieoptionen (falls E12) | A | b |
| `POST /nodes/{id}/update` | `{target: "agent" \| "os"}` | A | g |
| `GET /nodes/{id}/containers` | SmartCluster-verwaltete Container | U | c |
| `POST /nodes/{id}/containers/{service}/{start\|stop\|restart}` | Container-Aktion | A | c |
| `POST /nodes/{id}/logs/streams` | `{source}` → `{streamId}`; die Daten kommen per WS | U | c |
| `POST /nodes/{id}/remote-desktop/sessions` | `{rustdeskId, password, idServer, key}` | A | f |
| `GET /roles` · `GET /roles/{id}/versions/{v}` | Katalog, Manifest, Parameter-Schema | U | c |
| `POST /roles/{id}/versions/{v}/check` | `{nodeIds}` → Eignung pro Node mit Begründung | U | c |
| `GET /assignments` · `GET /assignments/{id}` | Rollen-Instanzen | U | c |
| `POST /assignments` | `{roleId, version, name, members[], params}` → `{assignment, jobId}` | A | c |
| `PATCH /assignments/{id}` | Parameter oder Member ändern → Job | A | c |
| `POST /assignments/{id}/upgrade` | `{version}` → Job | A | c |
| `POST /assignments/{id}/promote` | Replica zum Primary machen (manuelles Failover) → Job | A | d |
| `DELETE /assignments/{id}?deleteData=false` | entfernen → Job (Daten löschen nur explizit) | A | c |
| `GET /assignments/{id}/connection-info?reveal=` | Adressen und Zugangsdaten (`reveal` erzeugt Audit-Eintrag) | A | c |
| `GET /jobs` · `GET /jobs/{id}` · `POST /jobs/{id}/retry` · `POST /jobs/{id}/cancel` | Jobs | U / A | c |
| `GET /cluster` · `GET /cluster/topology` | Name, Modus, CA-Fingerprint; Graph für die Cluster-Ansicht | U | a / c |
| `GET /alerts` · `POST /alerts/{id}/ack` | Warnungen | U / A | a / b |
| `GET /audit` | Filter: Zeit, Akteur, Aktion | A | b |
| `GET /version` · `GET /ca.crt` · `GET /healthz` · `GET /readyz` | System | 🔓 | a |

**Agent-Endpunkte** (`https://<host>:8444/agent/v1`)

| Methode & Pfad | Zweck | Auth |
|---|---|---|
| `POST /enroll` | Kopplung (4.3), auch per Bootstrap-Code | Join-Code-Beweis |
| `GET /ws` | Befehls- und Telemetriekanal | mTLS |
| `POST /renew` | neues Zertifikat (CSR) | mTLS |
| `GET /bundles/{roleId}/{version}` | Rollen-Bundle + Signatur | mTLS |
| `GET /releases/{version}/{os}/{arch}` | Agent-Binary + Signatur | mTLS |

---

## 8. WebSocket-Protokoll

### 8.1 Allgemein
- Textframes mit JSON und der Hülle `{"t": "<typ>", "id": "<ulid, optional>", "ts": "<RFC 3339>", "d": {…}}`.
- Schemas liegen in `/docs/protocol/*.schema.json` und werden in den Tests beider Seiten gegen echte Nachrichten geprüft.
- Maximale Nachrichtengröße: 1 MiB. Bundles und Binaries laufen über HTTPS, nicht über WS.
- WS-Ping alle 20 s von der Kontrollzentrale. Ohne Nachricht seit 60 s gilt der Node als `offline`.

**Close-Codes**

| Code | Bedeutung |
|---|---|
| 4001 | Authentifizierung fehlgeschlagen |
| 4401 | Token abgelaufen |
| 4403 | keine Berechtigung |
| 4409 | ersetzt durch eine neuere Verbindung desselben Nodes |
| 4426 | Protokollversion nicht unterstützt |
| 4410 | Node entfernt bzw. Zertifikat gesperrt |

### 8.2 Agent ↔ Kontrollzentrale (`wss://<host>:8444/agent/v1/ws`, Subprotokoll `smartcluster.agent.v1`)

**Ablauf**
1. Der Agent verbindet sich und sendet `hello`.
2. Die Kontrollzentrale antwortet mit `hello.ok` oder schließt mit 4426/4410.
3. Danach laufen parallel: `stats` alle 5 s, `event` bei Änderungen, `cmd` jederzeit.

| Richtung | `t` | `d` (Auszug) |
|---|---|---|
| A → K | `hello` | `{protocol: 1, agent_version, node_id, os{name, version, build, eol}, hardware{vendor, model, bios, cpu, ram, disks[], gpus[], nics[]}, capabilities[], assignments[{id, version, status}]}` |
| K → A | `hello.ok` | `{protocol: 1, server_time, stats_interval_s: 5, min_agent_version, cmd_pubkeys[]}` |
| A → K | `stats` | siehe unten |
| A → K | `event` | `{kind: "container.state" \| "health" \| "power" \| "net.change", …}` |
| K → A | `cmd` | signierter Befehl (5.3) |
| A → K | `cmd.ack` | `{cmd_id}`: geprüft und angenommen |
| A → K | `cmd.reject` | `{cmd_id, code: "bad_signature" \| "expired" \| "replay" \| "unknown_type" \| "invalid_payload" \| "busy"}` |
| A → K | `cmd.progress` | `{cmd_id, step, pct, msg_key, msg_args}` |
| A → K | `cmd.result` | `{cmd_id, ok, error_code, data}` |
| A → K | `log` | `{stream_id, lines[{ts, src: "stdout" \| "stderr", text}]}` |

**`stats`-Paket** (Bytes als Ganzzahlen, fehlende Werte als `null`):
```json
{
  "ts": "2026-10-14T12:00:05Z",
  "cpu": { "pct": 12.5, "per_core": [10.1, 14.9], "load1": 0.42, "freq_mhz": 2400, "temp_c": null },
  "mem": { "total": 8589934592, "used": 3221225472, "swap_total": 2147483648, "swap_used": 0 },
  "disks": [{ "mount": "/", "total": 256060514304, "used": 41875931136, "read_bps": 0, "write_bps": 12288 }],
  "net": [{ "iface": "enp0s25", "up": true, "rx_bps": 5120, "tx_bps": 2048 }],
  "gpus": [{ "name": "NVIDIA GeForce 940MX", "util_pct": 0, "mem_used": 0, "mem_total": 2147483648, "temp_c": 41 }],
  "temps": [{ "sensor": "coretemp/Package id 0", "c": 52.0 }],
  "battery": { "present": true, "pct": 81, "on_ac": true, "health_pct": 64 },
  "uptime_s": 123456
}
```
**Backpressure:**
- Ist der Sendepuffer voll, verwirft der Agent alte `stats` und schickt nur das neueste.
- Logs sind auf 500 Zeilen/s und 8 KiB pro Zeile begrenzt. Was darüber hinausgeht, fasst der Agent als „n Zeilen ausgelassen“ zusammen.

**Offline-Verhalten:**
- Befehle für einen Node, der offline ist, bleiben im Job `queued`. Sie werden **beim Senden** neu signiert, deshalb läuft das 5-min-Fenster erst ab dem Senden.
- Ist ein Job abgebrochen, wird nichts mehr gesendet.

### 8.3 App ↔ Kontrollzentrale (`wss://<host>:8443/api/v1/live`)
- Die erste Nachricht ist `auth` mit dem Access-Token. Das Token steht **nie in der URL**. Die Antwort ist `auth.ok`.
- Nach einem Token-Refresh sendet die App erneut `auth`. Läuft das Token ab ohne neues `auth`, wird die Verbindung mit 4401 geschlossen.

| Richtung | `t` | `d` |
|---|---|---|
| App → K | `auth` | `{token}` |
| App → K | `sub` / `unsub` | `{topics: ["nodes", "node:<id>:stats", "job:<id>", "alerts", "logs:<streamId>"]}` |
| K → App | `node.status` | `{node_id, status, last_seen_at}` |
| K → App | `node.stats` | `{node_id, stats}` (nur an Abonnenten) |
| K → App | `job.update` | `{job_id, status, progress, step}` |
| K → App | `alert` | `{alert}` |
| K → App | `log` | `{stream_id, lines[]}` |

**Log-Stream:**
1. `POST …/logs/streams` liefert eine `streamId`.
2. Die Kontrollzentrale sendet `logs.subscribe` (signiert) an den Agent.
3. Die Zeilen gehen auf das Topic `logs:<streamId>`.
4. Bei `unsub` oder Verbindungsende folgt `logs.unsubscribe`.

---

## 9. Befehlstypen (Agent-seitig fest einprogrammiert)

| `type` | Payload | Wirkung | MS |
|---|---|---|---|
| `cert.renew` | `{}` | neuen CSR erzeugen und über `/renew` einreichen | a |
| `hwcheck.run` | `{}` | Hardware-Check, Ergebnis als `cmd.result` | b |
| `node.reboot` | `{firmware_setup: bool, delay_s}` | Neustart, optional ins Firmware-Setup | b |
| `node.wake` | `{mac}` | Magic Packet an den Subnetz-Broadcast senden, für einen anderen Node, der offline ist (ADR-0011) | b |
| `backup.store` | `{backup_id, sha256}` | Backup per mTLS holen, prüfen, ablegen, 7 Versionen behalten (ADR-0016) | d |
| `power.configure` | `{lid: "ignore", sleep_on_ac: false}` | Energieoptionen (E12) | b |
| `docker.ensure` | `{}` | Docker prüfen bzw. installieren | c |
| `role.apply` | `{assignment_id, role_id, version, bundle_sha256, member, params, secrets, peers}` | Bundle holen, prüfen, rendern, anwenden, Healthchecks | c |
| `role.remove` | `{assignment_id, delete_data: bool}` | Hooks, `compose down`, Daten nur bei `true` löschen | c |
| `container.action` | `{assignment_id, service, action: "start" \| "stop" \| "restart"}` | nur Container mit Label `smartcluster.assignment` | c |
| `logs.subscribe` | `{stream_id, source: "agent" \| {assignment_id, service}, tail: 200}` | Log-Stream starten | c |
| `logs.unsubscribe` | `{stream_id}` | Log-Stream beenden | c |
| `tunnel.open` | `{tunnel_id, mode: "listen" \| "connect", peer_node_id, peer_addr, local_port, target_port}` | mTLS-Tunnel für AI-RPC | e |
| `tunnel.close` | `{tunnel_id}` | Tunnel schließen | e |
| `rustdesk.ensure` | `{id_server, relay_server, key, rotate_password: bool}` | Client installieren und konfigurieren, Passwort setzen | f |
| `agent.update` | `{version, sha256}` | Selbst-Update (Hersteller-Signatur wird extra geprüft) | g |
| `os.update` | `{}` | nur Linux: `apt-get update` + `upgrade` mit festen Argumenten. Windows: nur Status melden | g |

`peers` enthält die Adressen der anderen Member desselben Assignments. Daraus erzeugt der Agent Umgebungsvariablen (`SC_PEERS_<MEMBER>=host:port,…`).

---

## 10. Rollen-Template-Format (`/roles`)

### 10.1 Aufbau
```
roles/
  index.yaml                       # Katalog (wird beim Release signiert)
  gameserver-minecraft/
    1.0.0/
      role.yaml                    # Manifest
      compose.yaml                 # Basis
      compose.velocity.yaml        # optional: Overlay bzw. weitere Member
      hooks/
        pre-apply.sh               # POSIX sh (Linux / WSL)
        post-apply.sh
      i18n/de.yaml, i18n/en.yaml   # Texte für UI und Fehler
      icon.svg
```
**Bundle:** `<id>-<version>.scrole`. Das ist ein tar.gz aus dem Versionsordner mit einer Datei `MANIFEST` (SHA-256 jeder Datei) und `MANIFEST.sig` (Ed25519 mit dem Hersteller-Rollen-Key). Der Agent prüft die Signatur und **jeden** Datei-Hash, bevor er irgendetwas ausführt. Entwicklungs-Bundles sind mit einem Dev-Key signiert, und Release-Builds des Agents lehnen sie ab.

### 10.2 `role.yaml` (Beispiel Minecraft)
```yaml
apiVersion: smartcluster/v1
kind: Role
id: gameserver-minecraft
version: 1.0.0
name:        { de: "Gameserver (Minecraft)", en: "Game server (Minecraft)" }
description: { de: "Minecraft Java Server (Paper oder Fabric)", en: "Minecraft Java server (Paper or Fabric)" }
category: gameserver

platforms: [linux/amd64, windows/amd64]          # windows = Linux-Container über WSL2 (1.1)

requirements:
  minRamMB: 3072
  minDiskGB: 10
  docker: true
  ports: [{ port: "{{params.port}}", proto: tcp }]

multiNode:
  mode: independent                               # none | load-balanced | replicated | split-model | independent
  label: { de: "Mehrere unabhängige Server", en: "Several independent servers" }
  explain:
    de: "Jede Welt läuft auf genau einem Rechner. Mehr Nodes = mehr Server, nicht mehr Leistung pro Welt."
    en: "Each world runs on exactly one machine. More nodes = more servers, not more power per world."

topology:
  members:
    - name: server
      count: { min: 1, max: 8 }
      compose: [compose.yaml]

params:
  - key: server_type
    type: enum
    values: [paper, fabric]
    default: paper
    label: { de: "Server-Typ", en: "Server type" }
  - key: mc_version
    type: string
    pattern: '^1\.[0-9]{1,2}(\.[0-9]{1,2})?$'
    default: "1.21.11"
  - key: memory_mb
    type: int
    min: 1024
    max: 32768
    default: 2048
    maxFromNode: "ram_total_mb - 1024"            # feste, eingebaute Regeln, keine freie Ausdruckssprache
  - key: port
    type: port
    default: 25565
  - key: eula_accepted
    type: bool
    required: true
    mustBe: true
    label: { de: "Ich akzeptiere die Minecraft-EULA", en: "I accept the Minecraft EULA" }
    link: "https://www.minecraft.net/eula"

secrets:
  - key: rcon_password
    generate: { length: 32, charset: alnum }

hooks:                                            # optional, alle mit Timeout
  preApply:  { linux: hooks/pre-apply.sh,  timeout: 120s }
  postApply: { linux: hooks/post-apply.sh, timeout: 300s }

healthchecks:
  - name: container
    type: container_healthy                       # nutzt den HEALTHCHECK des Images
    service: mc
    interval: 30s
    retries: 5
    startPeriod: 180s
  - name: port
    type: tcp
    port: "{{params.port}}"
    interval: 30s

connectionInfo:                                   # was die UI nach dem Start anzeigt
  - label: { de: "Serveradresse", en: "Server address" }
    value: "{{node.address}}:{{params.port}}"

upgrade:
  from: ">=1.0.0 <2.0.0"
```

### 10.3 `compose.yaml` (Beispiel)
```yaml
services:
  mc:
    image: itzg/minecraft-server@sha256:<gepinnt beim Release>
    restart: unless-stopped
    environment:
      EULA: "TRUE"                     # wird nur gerendert, wenn eula_accepted == true
      TYPE: "${SC_PARAM_SERVER_TYPE_UPPER}"
      VERSION: "${SC_PARAM_MC_VERSION}"
      MEMORY: "${SC_PARAM_MEMORY_MB}M"
      RCON_PASSWORD: "${SC_SECRET_RCON_PASSWORD}"
    ports:
      - "${SC_PARAM_PORT}:25565"
    volumes:
      - "${SC_DATA_DIR}/data:/data"
    labels:
      smartcluster.assignment: "${SC_ASSIGNMENT_ID}"
      smartcluster.service: "mc"
```

### 10.4 Regeln
- **Parametertypen:** `string` (Pflicht-Regex), `int` (min/max), `bool`, `enum`, `port`, `size_mb`. Dazu `secret`, das automatisch erzeugt und nie vom Nutzer eingegeben wird.
- **Rendering:**
  - Der Agent schreibt `.env` mit `SC_PARAM_*`, `SC_SECRET_*`, `SC_ASSIGNMENT_ID`, `SC_DATA_DIR`, `SC_NODE_ADDRESS` und `SC_PEERS_*`.
  - Abgeleitete Werte (etwa `_UPPER`) kommen aus einer festen, eingebauten Liste.
  - `{{…}}` gibt es nur in `role.yaml`, und nur für die dokumentierten Felder `params.*`, `node.*` und `peers.*`. Es gibt keine freie Template-Sprache.
- **Compose-Aufruf:** immer `docker compose -p sc-<assignment-id> -f … --env-file … up -d --remove-orphans`, mit festen Argumenten.
- **Overlays:** `overlays: [{ when: { capability: nvidia }, compose: compose.gpu.yaml }]`. Bedingungen gibt es nur aus einer festen Liste (`capability`, `member`, `param == wert`).
- **Hooks:**
  - Linux und WSL: POSIX sh. Native Windows-Hooks (PowerShell) nur, wenn eine Rolle sie braucht.
  - Arbeitsverzeichnis ist das Rollenverzeichnis.
  - Eingaben kommen nur als Umgebungsvariablen.
  - Jeder Hook hat ein Timeout, stdout/stderr gehen ins Job-Log.
  - Phasen: `preApply`, `postApply`, `preRemove`, `postRemove`, `preUpgrade`, `postUpgrade`.
- **Healthcheck-Typen:** `http` (url, erwartete Status), `tcp`, `container_running`, `container_healthy`, `exec` (feste argv im Container aus dem Template).
- **Versionen:** SemVer. `upgrade.from` legt fest, welche Versionen direkt aktualisiert werden dürfen. Ein Major-Wechsel braucht explizite Migrations-Hooks.
- **Daten:**
  - Linux: `/var/lib/smartcluster/roles/<assignment-id>/`
  - Windows: in der WSL-Distro
  - „Rolle entfernen“ behält die Daten standardmäßig. Löschen geht nur mit getippter Bestätigung, weil es nicht rückgängig zu machen ist.
- **CI-Prüfungen pro Template:** Schema-Validierung von `role.yaml`, `docker compose config`, `shellcheck` für Hooks, Verbotsliste (5.4), alle Images per Digest gepinnt, i18n vollständig für de und en.

---

## 11. Rollen v1 im Detail

### 11.1 Ehrlichkeitstabelle Multi-Node

| Rolle | Mehrere Nodes bedeutet | Lastverteilung? | Redundanz? | Min. Nodes | Grenzen |
|---|---|---|---|---|---|
| Kontrollzentrale | – (genau 1 Node in v1) | nein | nein | 1 | Fällt sie aus, laufen Dienste weiter, lassen sich aber nicht steuern |
| Website | 1 Einstiegs-Node (Caddy-LB) + n Backends | **ja**, für statische bzw. zustandslose Seiten | Backends ja; Einstiegs-Node bleibt Single Point of Failure | 2 | Inhalte müssen auf alle Backends (E13) |
| Gameserver | n unabhängige Server, optional verbunden über Velocity | **nein** | nein | 1 | eine Welt = ein Rechner |
| Datenbank Postgres | Primary + Streaming-Replicas | nur Lesen, falls die App das nutzt | **ja**, Failover manuell | 2 | Auto-Failover bräuchte ≥ 3 Nodes + Patroni/etcd |
| Datenbank MySQL | Primary + Replica (GTID, asynchron) | nur Lesen | ja, manuell | 2 | wie Postgres |
| Datenbank Redis/Valkey | Primary + Replica | nur Lesen | ja, manuell | 2 | Sentinel bräuchte 3 |
| Cloud-Storage Nextcloud | 1 Node | nein | nein | 1 | Multi-Node-Nextcloud braucht Shared Storage + DB + Redis, kommt nicht in v1 |
| Cloud-Storage Garage (S3) | Daten 2- oder 3-fach auf Nodes repliziert | Lesen verteilt | **ja** | 3 empfohlen (2 möglich) | Nutzbarer Platz ≈ Summe ÷ Replikationsfaktor |
| AI Single | Ollama auf 1 Node | – | – | 1 | – |
| AI Multi | llama.cpp: Modellschichten auf Nodes verteilt | **nein**, meist langsamer | **nein**, fällt ein Node aus, steht das Modell | 2 | LAN-Kabel nötig, kein WLAN; langes Laden über das Netz |

### 11.2 Rollen

**Kontrollzentrale (`control`)**
- Container: `control`, `postgres`, `rustdesk-hbbs`, `rustdesk-hbbr` (Abschnitt 3.2).
- Ist im Agent eingebettet und wird ausschließlich über `init` bzw. Upgrade angewendet.

**Website (`website`)**
- Image: `caddy:2`, gepinnt. Die Caddyfile nutzt nur `{$ENV}`-Platzhalter, und Domain-Parameter sind per Regex abgesichert.
- **Member `entry`:** Caddy als Reverse Proxy und Load Balancer (`reverse_proxy` mit mehreren Upstreams und aktiven Healthchecks). Optional leitet er auch auf Dienste anderer Rollen weiter, etwa Nextcloud, unter Subdomain oder Pfad.
- **Member `backend`:** Caddy `file_server`.
- **TLS:**
  - nur LAN: `tls internal`, die Caddy-eigene CA
  - mit öffentlicher Domain und Portfreigabe: automatisches ACME
- **Inhalt (ADR-0013):** ZIP-Upload in der UI (statisch, mit Prüfung gegen Zip-Slip und Zip-Bomben, inhaltsadressiert). Der Agent holt den Inhalt bei `role.apply` per mTLS und schaltet atomar um. Git-URL kommt später.

**Gameserver Minecraft (`gameserver-minecraft`)**
- `itzg/minecraft-server`, gepinnt, mit Paper oder Fabric.
- Parameter: Version, RAM, Port. EULA-Zustimmung durch den Nutzer ist Pflicht; SmartCluster setzt sie **nie** automatisch.
- Optional Member `proxy`: `itzg/mc-proxy` mit `TYPE=VELOCITY` (E6).

**Datenbank (`database-postgres`, `database-mysql`, `database-redis`)**
- Images: `postgres:18`, `mysql:8.4` (LTS) und `valkey/valkey` (BSD-Lizenz, Redis-kompatibel; E21).
- Keine Bitnami-Images, denn der kostenlose Bitnami-Katalog ist seit 2025 eingeschränkt.
- Replikation wird über Hooks eingerichtet: `pg_basebackup` bzw. GTID-Replikation.
- TLS mit Zertifikaten der Cluster-CA.
- Zugangsdaten werden als Secrets erzeugt und in `connectionInfo` angezeigt.

**Cloud-Storage (`storage-nextcloud`, `storage-s3`)**
- **Nextcloud:** Image `nextcloud` (apache), mit eigener Postgres- und Valkey-Instanz in derselben Rolle, damit sie unabhängig von der DB-Rolle bleibt. Parameter: Admin-Konto (Secret), `trusted_domains`.
- **Garage:**
  - Jeder Node ist eine eigene Zone.
  - `garage layout assign/apply` läuft über Hooks per `docker exec` mit fester argv.
  - Parameter: Replikationsfaktor (2 oder 3, höchstens so viele wie Nodes), Kapazität pro Node.
  - Ergebnis: ein Bucket plus Access-Key in `connectionInfo`.

**AI (`ai-ollama`, `ai-llamacpp-cluster`)**
- **Single (Ollama):**
  - Image `ollama/ollama`, gepinnt.
  - NVIDIA-GPU wird automatisch erkannt. Der Hook installiert dann das `nvidia-container-toolkit` (nur Linux).
  - Modelle kommen aus einer kuratierten Liste mit RAM-Bedarf. Ungeeignete Modelle sind für den Node ausgegraut.
  - Die API liegt im LAN auf Port 11434 und ist OpenAI-kompatibel unter `/v1`.
- **Multi (llama.cpp):**
  - Member `coordinator` (`llama-server --rpc …`) und `worker` (`rpc-server -H 127.0.0.1`).
  - Der Coordinator-Agent öffnet pro Worker einen lokalen Tunnel-Port, der über mTLS zum Worker-Agent auf :8446 und weiter zu `127.0.0.1:<rpc>` geht.
  - Das Modell (GGUF, mit gepinntem SHA-256 aus der kuratierten Liste) liegt auf dem Coordinator.
  - `--tensor-split` wird proportional zum freien RAM der Nodes gesetzt. Die UI zeigt den Anteil pro Node und „gemeinsam ausgeführt“.
  - Mindestversion: ≥ b8492 und mit Fix für CVE-2026-39909. Die genaue Build-Nummer wird in Meilenstein e verifiziert. Wenn nötig, bauen wir ein eigenes Image.
  - Die API ist ebenfalls OpenAI-kompatibel, sodass die UI für Single und Multi denselben Endpunkt-Typ zeigt.
- **Chat (ADR-0005):** Member `ui` mit Open WebUI, unverändert und mit Branding. Standardmäßig auf dem Ollama- bzw. Koordinator-Node, angebunden an die OpenAI-kompatible API.

---

## 12. Remote-Desktop

**Aufbau**
- **Server:** `rustdesk-hbbs` und `rustdesk-hbbr` sind Teil der Rolle `control`. hbbs erzeugt sein Schlüsselpaar selbst. Die Kontrollzentrale liest den öffentlichen Schlüssel aus und verteilt ihn an die Clients.
- **Node:** Der Agent installiert den RustDesk-Client in einer gepinnten Version mit geprüftem SHA-256:
  - Windows: `--silent-install`
  - Linux: das .deb, nur mit Desktop
- **Konfiguration auf dem Node:** eigener ID-Server, Relay und Key. Dazu ein zufälliges dauerhaftes Passwort (`--password`). Die ID kommt per `--get-id` und wird gemeldet. Das Passwort wird verschlüsselt in der Kontrollzentrale gespeichert.
- **Haupt-PC:** Der Electron-Installer bringt den unveränderten RustDesk-Client mit Lizenztext und Quellcode-Link mit, oder er lädt ihn herunter (Entscheidung in Meilenstein f).

**Session-Ablauf**
1. Ein Admin klickt „Remote-Desktop“.
2. `POST …/remote-desktop/sessions` erzeugt einen Audit-Eintrag und liefert ID, Passwort und Serverdaten.
3. Electron startet den Client mit `--connect <id>`.
4. Das Passwort wird automatisch übergeben, falls das geht (Spike), sonst per Kopier-Knopf.
5. Wenn die CLI es zulässt, wird das Passwort nach der Session rotiert (Spike).

**Plattformgrenzen**
- **Windows:** Der Dienstmodus funktioniert inklusive Anmeldebildschirm und UAC-Dialogen.
- **Linux:** nur mit X11-Sitzung (1.6). Wayland kommt, sobald unbeaufsichtigter Zugriff in einer stabilen RustDesk-Version verfügbar ist.
- **Ohne Desktop:** nicht möglich.

**Netz:** Alles bleibt im LAN. Zugriff von unterwegs bräuchte Portfreigaben für 21115–21119 und gehört zu E14.

---

## 13. Lizenzsystem

**Format** (Vorschlag)
```
SC1.<base64url(payload)>.<base64url(ed25519-signatur)>
payload = {"lid":"L-2026-000123","sub":"kunde@example.com","ed":"<edition>",
           "max_nodes":5,"feat":["ai_multi","remote_desktop"],
           "iat":1792000000,"exp":null}
```
- **Validierung:** Die Kontrollzentrale prüft offline gegen den einkompilierten öffentlichen Lizenz-Key. Das passiert bei der Aktivierung und danach täglich (Status, Ablauf).
- **Erzeugung:** `tools/licensegen` läuft nur beim Hersteller. Der private Schlüssel liegt nie im Repository und nie in einem ausgelieferten Binary.

**Durchsetzung** (Vorschlag)
- Laufende Dienste werden **nie** gestoppt.
- Bei abgelaufener oder fehlender Lizenz sind gesperrt: neue Nodes über `max_nodes` hinaus, neue Rollen-Zuweisungen und lizenzpflichtige Features.
- Anzeigen, Logs, Remote-Desktop zur Fehlersuche und das Entfernen von Rollen bleiben möglich.

**Grenzen:** siehe 1.13.
**Editionen (ADR-0010):** Free (1 Node, ohne AI Multi-Node und Remote-Desktop, kein Schlüssel nötig), Home (bis 5 Nodes, alles), Pro (unbegrenzt). Eine 14-Tage-Trial im Umfang von Home startet mit der Cluster-Erstellung. Online-Aktivierung kommt später.

---

## 14. Installer, Verteilung, Updates

**Windows (.exe)**
- Inno Setup mit `PrivilegesRequired=admin` (UAC), Authenticode-signiert (E18).
- Seiten:
  1. OS-Prüfung (Win 10 22H2 bzw. Win 11, x64)
  2. „Neuen SmartCluster starten“ oder „Beitreten“ (mit Code und optionaler Adresse)
  3. Zustimmung zu Energieoptionen (E12)
  4. Installation nach `C:\Program Files\SmartCluster\`, dann `service install`
  5. Ergebnis: Setup-Code bzw. „Erfolgreich beigetreten“

**Linux (.deb)**
- Gebaut mit nfpm. Inhalt: Binary in `/usr/bin`, systemd-Unit, `postinst` aktiviert den Dienst (Zustand `unpaired`) und fragt **nichts** interaktiv.
- Danach `sudo smartcluster-agent init` oder `join <CODE>`.
- Verteilt wird die .deb über die Kontrollzentrale (`/downloads`). Ein APT-Repository → E19.

**Geführte Linux-Installation** (Kontroll-App auf dem Haupt-PC, Meilenstein g)
1. „Alten Rechner mit Linux einrichten“ → Debian 13 netinst wird heruntergeladen, SHA256SUMS und GPG-Signatur werden geprüft.
2. Ein USB-Stick wird mit Preseed geschrieben. Darin stecken: Agent-.deb, Join-Code, optional ein Desktop (XFCE/X11, für Remote-Desktop), Energieoptionen.
3. Dazu eine Anleitung zur Boot-Menü-Taste aus der BIOS-Guide-DB.

**⚠ Die Installation löscht die gesamte Festplatte des Zielrechners.** Die App verlangt deshalb eine getippte Bestätigung und zeigt den Hinweis mehrfach.
Technische Details klärt ein Spike in g: wie der Preseed neben das Hybrid-ISO kommt, und ob UEFI und Legacy-BIOS beide funktionieren. Debian statt Ubuntu, weil es auf alter Hardware weniger RAM braucht.

**Updates**
- Release-Manifest (Version, SHA-256 pro Datei, Hersteller-Signatur) von einer Update-Quelle des Herstellers (E19).
- Die Kontrollzentrale lädt Agent-Binaries und Rollen-Bundles herunter und verteilt sie. Jeder Agent prüft die Signatur selbst.
- Die Kontrollzentrale selbst wird per Rollen-Upgrade aktualisiert (neue Image-Digests), das der Agent auf ihrem Node ausführt.
- **OS-Updates:**
  - Linux: über `os.update`
  - Windows: Windows Update bleibt zuständig. Der Agent meldet nur „Neustart ausstehend“.

---

## 15. Monorepo, Tooling, CI, Tests

```
smartcluster/
  ARCHITECTURE.md · README.md · go.work · .editorconfig
  agent/        Go-Modul: cmd/smartcluster-agent, internal/…, packaging/{windows,linux}
  control/      Go-Modul: cmd/smartcluster-control, internal/…, migrations/, biosguides/, web/ (Build-Ausgabe der App, eingebettet)
  pkg/          Go-Modul, gemeinsam: proto, sign, template, version          ← NEU, braucht OK (E17)
  app/          pnpm-Workspace: src/renderer (React), src/main + src/preload (Electron)
  roles/        Templates (Abschnitt 10) + index.yaml
  tools/        licensegen, rolesign, releasesign (nur Hersteller, nie ausgeliefert)  ← NEU (E17)
  docs/         api/openapi.yaml, protocol/*.schema.json, adr/, Benutzer-Doku
  .github/workflows/                                                        ← NEU (E17)
```

**Qualitätsregeln**
- **Go:** `gofmt`, `go vet`, `golangci-lint` (u. a. errcheck, staticcheck, gosec), `go test -race`.
- **TS:** `strict: true` plus `noUncheckedIndexedAccess`, ESLint (typescript-eslint strict), Prettier, Vitest.
- **README pro Paket:** Zweck, Build, Test, Konfiguration.
- **ADRs:** Größere Entscheidungen kommen als ADR in `/docs/adr`. Die Entscheidungen aus Abschnitt 17 werden dort festgehalten.

**CI** (GitHub Actions in einem privaten Repo, E18)

| Job | Inhalt |
|---|---|
| `go` | vet, lint, `test -race`, Builds windows/linux amd64 |
| `app` | `tsc --noEmit`, ESLint, Vitest, Build Web und Electron |
| `contracts` | OpenAPI und WS-Schemas validieren; generierter Code muss aktuell sein (Diff = Fehler) |
| `roles` | Template-Prüfungen aus 10.4 |
| `integration` | Postgres über testcontainers-go; `control` + 2 Agents in Containern (Docker-in-Docker für Rollen-Tests) |
| `e2e` | Playwright gegen die Web-UI mit simulierten Agents |
| `release` (nur Tags, manuelle Freigabe) | Signieren (Hersteller-Keys nur in dieser geschützten Umgebung), Inno Setup auf Windows-Runner, nfpm, Container-Images |

**Testarten pro Ebene**
- Unit: Stats-Collector mit Fake-Quellen, Signatur und Replay-Prüfung, Template-Parser, Auth.
- Vertrag: Nachrichten gegen JSON-Schema.
- Integration: Enrollment → Stats → Befehl → Ergebnis.
- E2E: Wizard, Dashboard, Rolle zuweisen.
- Echte Hardware: Ein manueller Testplan pro Meilenstein auf mindestens einem Windows- und einem Linux-Laptop.

---

## 16. Meilensteine (jeweils lauffähig und mit Tests)

| MS | Umfang | Abnahme |
|---|---|---|
| **a** | Agent (Linux + Windows): Dienst, Enrollment per Join-Code (Code per CLI der Kontrollzentrale), mTLS-WS, `stats` alle 5 s, `hello`/Offline-Erkennung. Kontrollzentrale: Postgres, Registry, Timeseries + Rollup, App-WS. App: Dashboard + Node-Detail mit Live-Graphen (Web + Electron). **Spike Windows-Docker (1.1).** | 2 echte Rechner erscheinen live im Dashboard; Ziehen des Netzkabels → `offline` nach ≤ 60 s; alle CI-Jobs grün. *Hinweis: Bis b ist die UI ohne Login – nur Entwicklung, kein Release.* |
| **b** | Auth (Setup-Code, Admin, Einladungen, JWT + Refresh, Rechte), Audit-Log, Onboarding-Wizard bis „Rolle wählen“, Hardware-Check + BIOS-Guides (erste Hersteller) + „Neustart ins BIOS“, Reboot, „Aufwecken“ (WoL), Energieoptionen, Wiederherstellungsschlüssel im Wizard, Warnungen | Wizard vom leeren System bis „Node bereit“ ohne Terminal; Login-Rate-Limit getestet; Audit-Einträge für alle Aktionen |
| **c** | Rollen-Engine, Bundle-Signierung, Jobs, `docker.ensure`, Container-Aktionen, Live-Logs, Rollen **Website** (ZIP-Upload, Reverse Proxy, LB) und **Gameserver (Minecraft)** (unabhängig + optional Velocity), Windows-Engine (`engine-keeper`, falls der Spike erfolgreich ist) | Minecraft per Knopf auf einem Node, Client kann joinen; Website mit 2 Backends + LB; manipuliertes Bundle wird abgelehnt (Test) |
| **d** | **Datenbank** (Postgres, MySQL, Valkey; Replikation + manuelles Promote), **Cloud-Storage** (Nextcloud, Garage), **Backup** der Kontrollzentrale mit Restore-Test | Replica übernimmt nach Promote; Nextcloud-Upload funktioniert; Garage übersteht Ausfall eines von 3 Nodes |
| **e** | **AI** Single (Ollama) → Multi (llama.cpp RPC + mTLS-Tunnel) | Modell antwortet über die API; Multi: Modell, das nicht in einen Node passt, läuft über 2 Nodes; RPC-Port von außen nicht erreichbar (Test) |
| **f** | **Remote-Desktop** (hbbs/hbbr, Client-Installation, Session aus der App) | Windows- und Linux-(X11-)Node fernsteuerbar inkl. Neustart und Wiederverbinden |
| **g** | **Lizenzsystem**, **Installer** (.exe signiert, .deb), geführte Linux-Installation, Update-Mechanismus (Agent, Rollen, Kontrollzentrale) | Frischer Rechner → Cluster nur mit Installer/USB-Stick; abgelaufene Lizenz sperrt Neues, stoppt nichts; Update mit Rollback getestet |

---

## 17. Entscheidungen

Alle Entscheidungen wurden am 2026-10-14 getroffen. Die Begründungen stehen als ADRs in [`docs/adr`](docs/adr/README.md). Bei Widersprüchen gilt das ADR.

| # | Entscheidung | ADR |
|---|---|---|
| E1 | Backend in Go | [0001](docs/adr/0001-backend-in-go.md) |
| E2 | Weg B (Engine in WSL2 ohne Login) nach Praxistest, Fallback A; Wizard empfiehlt Linux | [0002](docs/adr/0002-windows-container-runtime.md) |
| E3 | Windows 10 mit Warnung bis 12.10.2027 | [0003](docs/adr/0003-windows-10-unterstuetzung.md) |
| E4 | Nextcloud + Garage | [0004](docs/adr/0004-cloud-storage-nextcloud-garage.md) |
| E5 | llama.cpp RPC; Open WebUI als Chat | [0005](docs/adr/0005-ai-llamacpp-rpc-open-webui.md) |
| E6 | Unabhängige Gameserver, optional Velocity | [0006](docs/adr/0006-gameserver-multi-node.md) |
| E7 | Setup-Code + Einladungen | [0007](docs/adr/0007-registrierung-setup-code-einladungen.md) |
| E8 | User nur lesend | [0008](docs/adr/0008-rechte-user-read-only.md) |
| E9 | Kein QR; Join-Code + mDNS | [0009](docs/adr/0009-join-code-mdns-kein-qr.md) |
| E10 | Offline-Keys; Free (1 Node, ohne AI-Multi/RD), Home (5 Nodes), Pro (unbegrenzt), 14-Tage-Trial | [0010](docs/adr/0010-lizenzmodell.md) |
| E11 | „Aufwecken“-Knopf | [0011](docs/adr/0011-wake-on-lan-aufwecken.md) |
| E12 | Energieoptionen + Akku-Hinweis | [0012](docs/adr/0012-energieoptionen-akku.md) |
| E13 | ZIP-Upload + Reverse Proxy; Git-URL später | [0013](docs/adr/0013-website-inhalt-zip-reverse-proxy.md) |
| E14 | Nur LAN + Anleitung | [0014](docs/adr/0014-lan-only.md) |
| E15 | CA-Download, kein Hersteller-DNS | [0015](docs/adr/0015-browser-tls-ca-download.md) |
| E16 | Tägliches Backup auf zweiten Node | [0016](docs/adr/0016-backup-kontrollzentrale.md) |
| E17 | `/pkg`, `/tools`, `/.github` | [0017](docs/adr/0017-monorepo-struktur.md) |
| E18 | GitHub Actions privat; Authenticode später | [0018](docs/adr/0018-ci-und-code-signing.md) |
| E19 | Private GitHub Releases, kein APT-Repo | [0019](docs/adr/0019-release-verteilung.md) |
| E20 | Kontroll-App für Windows + Linux | [0020](docs/adr/0020-plattformen-kontroll-app.md) |
| E21 | Valkey | [0021](docs/adr/0021-valkey-statt-redis.md) |

---

## 18. Quellen (Stand 2026-10-14)

- MinIO-Status: [MinIO CE in 2026: Retired Upstream, Source-Only](https://glukhov.org/data-infrastructure/object-storage/minio-dead/), [MinIO in Maintenance Mode (ayedo)](https://ayedo.de/en/posts/minio-im-maintenance-mode/), [MinIO Is Dead, Long Live MinIO](https://blog.vonng.com/en/db/minio-resurrect/index.md)
- Garage: [Garage Repository (Deuxfleurs)](https://git.deuxfleurs.fr/Deuxfleurs/garage/src/branch/list-full-pages), [Garage Quickstart](https://www.glukhov.org/data-infrastructure/object-storage/garage-quickstart/)
- exo / Ollama Multi-Node: [exo Docs](https://docsearch.algolia.com/mcp/docs/repo/exo-explore/exo), [exo Cluster Guide](https://bestllmfor.com/guide/exo-cluster-llm-plusieurs-machines/), [Ollama distributed inference: what is possible](https://dev.to/runcai/ollama-distributed-inference-what-is-possible-what-is-not-and-when-to-move-beyond-local-serving-2ajl)
- llama.cpp RPC: [Arm Learning Path – distributed inference](https://learn.arm.com/learning-paths/servers-and-cloud-computing/distributed-inference-with-llama-cpp/how-to-2/), [CVE-2026-34159 (OSV)](https://osv.dev/vulnerability/CVE-2026-34159), [CVE-2026-39909](https://cve.halosecurity.com/cve-advisory/cve-2026-39909-llama-cpp-rpc-server-use-after-free-vulnerability)
- Docker Desktop ohne Login: [Docker Forum – start without login](https://forums.docker.com/t/how-to-automatically-start-docker-in-windows-10-without-user-login/140086), [Docker Docs – Windows permission requirements](https://docs.docker.com/desktop/setup/install/windows-permission-requirements/)
- RustDesk: [Client-Doku (CLI-Flags)](https://rustdesk.com/docs/en/client/), [GitHub Issue #69 (`--connect`)](https://github.com/rustdesk/rustdesk/issues/69), [Wayland-Unterstützung (AlternativeTo, 08/2026)](https://alternativeto.net/news/2026/8/rustdesk-brings-true-unattended-remote-access-to-wayland-including-multi-monitor-setups/), [Pro-Lizenz-Doku](https://rustdesk.com/docs/en/self-host/rustdesk-server-pro/license/)
- Windows 10 ESU: [BleepingComputer – ESU bis Oktober 2027](https://bleepingcomputer.com/news/microsoft/microsoft-quietly-extends-free-windows-10-esu-support-to-october-2027), [TechRadar](https://www.techradar.com/computing/windows/microsoft-extends-windows-10-support-out-of-the-blue-consumers-now-get-updates-for-another-year-to-october-2027)
- Aus eigenem Wissen, beim jeweiligen Meilenstein gegen Primärquellen zu prüfen: Go ≥ 1.21 nur Windows 10+, Debian 13 ohne i386-Installer, WinRing0-Erkennung durch Defender, Bitnami-Katalogänderung 2025, Redis-8-Lizenz, Docker-Desktop-Lizenzschwellen, OWASP-Argon2id-Parameter.
