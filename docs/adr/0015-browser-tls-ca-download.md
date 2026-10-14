# ADR-0015: Browser-TLS: CA-Download statt Hersteller-DNS

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Ohne eigene Domain hat die Kontrollzentrale kein öffentlich vertrauenswürdiges Zertifikat. Die Electron-App pinnt die Cluster-CA und zeigt keine Warnung, ein normaler Browser schon.

## Entscheidung
- Die Web-UI bietet unter `GET /api/v1/ca.crt` die Cluster-CA zum Download an, mit Anleitung pro Betriebssystem und Browser (Windows-Zertifikatsspeicher, Firefox, Linux).
- Der Fingerprint wird angezeigt, damit man ihn vor dem Import vergleichen kann.
- Ein **DNS-Dienst des Herstellers gibt es nicht.**

## Konsequenzen
- Bis zum Import sieht die reine Web-UI die Browser-Warnung. Die Electron-App ist der empfohlene Weg.
- **Sicherheitshinweis in der Anleitung:** Wer die CA importiert, vertraut ihr für alle Domains. Der private Schlüssel der CA liegt verschlüsselt in der Kontrollzentrale. Wer sie kompromittiert, könnte Zertifikate für beliebige Domains ausstellen, denen dieser Browser dann vertraut.
  Gegenmaßnahme: Die CA bekommt Name Constraints (nur private IP-Bereiche, `.local` und die konfigurierten Hostnamen). Wo Clients sie auswerten, beschränkt das den Schaden.

## Aufgaben
1. [ ] CA mit Name Constraints erzeugen (Meilenstein a).
2. [ ] Download-Seite und Anleitungen (Meilenstein b).
