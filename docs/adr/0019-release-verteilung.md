# ADR-0019: Releases über private GitHub Releases, kein APT-Repo

**Status:** Akzeptiert (für die Anfangsphase)
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Installer, Agent-Binaries, Rollen-Bundles und Container-Images brauchen eine Ablage. Für die Anfangsphase soll es einfach sein.

## Entscheidung
- Releases liegen in **privaten GitHub Releases** des Repositorys.
- Ein **APT-Repository gibt es in v1 nicht.** Die `.deb` lädt man herunter oder bekommt sie von der Kontrollzentrale (`/downloads`).

## Konsequenzen
- **Wichtig:** Eine Kontrollzentrale beim Kunden kann private Releases nicht herunterladen. Dafür bräuchte sie ein GitHub-Token, und ein in Produkte eingebautes Token wäre ein Leck für alle. **Es werden keine Tokens ins Produkt eingebaut.**
- Deshalb gibt es in dieser Phase keine automatischen Online-Updates. Updates kommen als signierte Update-Datei, die ein Admin in der UI einspielt. Die Kontrollzentrale verteilt sie an die Agents, und jeder Agent prüft die Ed25519-Signatur.
- Das funktioniert auch in Clustern ohne Internet und bleibt später erhalten.
- **Vor dem ersten externen Release** muss eine öffentlich lesbare Download-Quelle feststehen (neues ADR), etwa öffentliche Release-Assets oder ein CDN. Erst dann kommt eine automatische Update-Prüfung.
- Container-Images (z. B. `control`) brauchen ebenfalls eine Quelle. In der Anfangsphase werden sie als `docker save`-Archiv in die Update-Datei bzw. den Installer gepackt.

## Aufgaben
1. [ ] Format der Update-Datei (signiertes Manifest + Artefakte) und „Update einspielen“ in der UI (Meilenstein g).
2. [ ] Neues ADR zur öffentlichen Download-Quelle vor dem ersten externen Release.
