# ADR-0005: AI: llama.cpp RPC für Multi-Node, Open WebUI als Chat

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
- Ollama kann ein Modell nicht über mehrere Rechner verteilen.
- exo konzentriert sich auf Apple Silicon/MLX und unterstützt Windows nicht offiziell.
- llama.cpp RPC verteilt die Schichten eines Modells auf mehrere Rechner. Es ist aber unauthentifiziert, und 2026 gab es dazu kritische CVEs (CVE-2026-34159, CVE-2026-39909).
- Ohne Chat-Oberfläche liefert die AI-Rolle nur eine API, und damit können Einsteiger nichts anfangen.

## Entscheidung
- **Single-Node:** Ollama.
- **Multi-Node:** `llama-server` als Koordinator, `rpc-server` auf den Workern.
  - `rpc-server` bindet nur an `127.0.0.1`.
  - Der Verkehr zwischen Nodes läuft ausschließlich durch den mTLS-Tunnel des Agents (:8446).
  - Die Mindestversion pinnt das Template: ≥ b8492 und mit Fix für CVE-2026-39909.
- **Chat:** Open WebUI als eigener Member `ui` der AI-Rolle. Standardmäßig läuft er auf dem Ollama- bzw. Koordinator-Node, der Nutzer kann einen anderen wählen. Open WebUI spricht die OpenAI-kompatible API von Ollama bzw. `llama-server` an.

## Betrachtete Optionen

| Kriterium | llama.cpp RPC | exo | mehrere Ollama + Load Balancer |
|---|---|---|---|
| Ein Modell über mehrere Rechner | ja | ja (Apple-Fokus) | nein |
| Windows/Linux x86 | ja | Windows nein | ja |
| Sicherheit | erst mit Tunnel und Pinning vertretbar | unklar | gut |
| Ehrlicher Nutzen | größere Modelle, nicht schneller | – | mehr gleichzeitige Anfragen |

## Konsequenzen
- Die UI-Beschriftung lautet „gemeinsam ausgeführt (größeres Modell, nicht schneller)“. WLAN-Nodes werden für Multi-Node ausgegraut.
- **Lizenz von Open WebUI:** Seit 2025 gilt eine eigene Lizenz mit Branding-Klausel. Wir liefern Open WebUI unverändert und mit sichtbarem Branding aus, ein White-Label gibt es nicht. Die genauen Bedingungen werden in Meilenstein e gegen den Lizenztext geprüft.
- Open WebUI braucht viel Platz (großes Image) und RAM. Die Rollen-Anforderungen weisen das aus, und auf schwachen Nodes ist der Member ausgegraut.
- Open WebUI hat eigene Benutzerkonten. Eine automatische Anmeldung mit SmartCluster-Konten gibt es in v1 nicht. SmartCluster legt das erste Open-WebUI-Admin-Konto an, falls das Image das unterstützt (prüfen in e), sonst zeigt die UI eine Anleitung.
- Ein Modellwechsel auf mehreren Nodes lädt das Modell über das LAN neu, das dauert Minuten.

## Aufgaben
1. [ ] Ollama und Open WebUI auf einem Node (Meilenstein e, Teil 1).
2. [ ] `tunnel.open/close` im Agent und `ai-llamacpp-cluster` (Meilenstein e, Teil 2).
3. [ ] Lizenztext von Open WebUI und die exakte llama.cpp-Mindestversion verifizieren.
4. [ ] Test, dass der RPC-Port von außen nicht erreichbar ist.
