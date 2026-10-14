# Architecture Decision Records

Jede grundlegende Entscheidung steht als eigenes ADR in diesem Ordner. Ein ADR wird nicht nachträglich umgeschrieben. Ändert sich eine Entscheidung, entsteht ein neues ADR, und das alte bekommt den Status „Ersetzt durch ADR-xxxx“.

Format: Kontext → Entscheidung → betrachtete Optionen → Konsequenzen → Aufgaben.

| ADR | Titel | Status |
|---|---|---|
| [0001](0001-backend-in-go.md) | Backend der Kontrollzentrale in Go | Akzeptiert |
| [0002](0002-windows-container-runtime.md) | Linux-Container auf Windows-Nodes | Akzeptiert, Praxistest offen |
| [0003](0003-windows-10-unterstuetzung.md) | Windows 10 mit Warnung bis ESU-Ende | Akzeptiert |
| [0004](0004-cloud-storage-nextcloud-garage.md) | Cloud-Storage: Nextcloud + Garage statt MinIO | Akzeptiert |
| [0005](0005-ai-llamacpp-rpc-open-webui.md) | AI: llama.cpp RPC für Multi-Node, Open WebUI als Chat | Akzeptiert |
| [0006](0006-gameserver-multi-node.md) | Gameserver: unabhängige Server, optional Velocity | Akzeptiert |
| [0007](0007-registrierung-setup-code-einladungen.md) | Registrierung: Setup-Code + Einladungen | Akzeptiert |
| [0008](0008-rechte-user-read-only.md) | Rechte: User nur lesend | Akzeptiert |
| [0009](0009-join-code-mdns-kein-qr.md) | Node-Kopplung: Join-Code + mDNS, kein QR | Akzeptiert |
| [0010](0010-lizenzmodell.md) | Lizenzmodell: Offline-Keys, Free/Home/Pro, 14-Tage-Trial | Akzeptiert |
| [0011](0011-wake-on-lan-aufwecken.md) | Wake-on-LAN mit „Aufwecken“-Knopf | Akzeptiert |
| [0012](0012-energieoptionen-akku.md) | Energieoptionen und Akku-Hinweis | Akzeptiert |
| [0013](0013-website-inhalt-zip-reverse-proxy.md) | Website: ZIP-Upload + Reverse Proxy | Akzeptiert |
| [0014](0014-lan-only.md) | v1 nur im LAN erreichbar | Akzeptiert |
| [0015](0015-browser-tls-ca-download.md) | Browser-TLS: CA-Download statt Hersteller-DNS | Akzeptiert |
| [0016](0016-backup-kontrollzentrale.md) | Tägliches Backup der Kontrollzentrale auf zweiten Node | Akzeptiert |
| [0017](0017-monorepo-struktur.md) | Monorepo-Struktur mit `/pkg`, `/tools`, `/.github` | Akzeptiert |
| [0018](0018-ci-und-code-signing.md) | CI über GitHub Actions, Authenticode später | Akzeptiert |
| [0019](0019-release-verteilung.md) | Releases über private GitHub Releases, kein APT-Repo | Akzeptiert |
| [0020](0020-plattformen-kontroll-app.md) | Kontroll-App für Windows + Linux | Akzeptiert |
| [0021](0021-valkey-statt-redis.md) | Valkey statt Redis | Akzeptiert |
