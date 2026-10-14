# ADR-0003: Windows 10 mit Warnung bis ESU-Ende

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Der reguläre Support für Windows 10 endete am 14.10.2025. Microsoft hat die Consumer-ESU bis zum 12.10.2027 verlängert. Windows 8 ist ausgeschlossen, und aktuelle Go-Versionen laufen dort ohnehin nicht.

## Entscheidung
Windows 10 22H2 x64 wird unterstützt.
- Installer, Hardware-Check und Dashboard zeigen eine dauerhafte Warnung mit Enddatum und einem Hinweis auf die geführte Linux-Installation.
- Ab dem 12.10.2027 wird Windows 10 behandelt wie Windows 8: keine Neuinstallation mehr, bestehende Nodes laufen mit roter Warnung weiter.

## Betrachtete Optionen
- **Unterstützen mit Warnung (gewählt):** Viele alte Laptops haben Windows 10. Ein harter Ausschluss würde Einsteiger sofort zum Umstieg auf Linux zwingen.
- **Sofort ausschließen:** konsequent zu Windows 8, aber schlechter Einstieg ins Produkt.

## Konsequenzen
- Windows 10 hat kein mirrored networking. Für Workload-Rollen gilt der NAT-Pfad mit portproxy (ADR-0002), also ein zusätzlicher Testpfad.
- Die Warnung braucht das Enddatum als Konfiguration. Es wird nicht fest einkompiliert, weil Microsoft Termine verschiebt.

## Aufgaben
1. [ ] OS-Support-Tabelle mit Enddaten in `/pkg/version` (Meilenstein a).
2. [ ] Warnung in Installer, Hardware-Check und Dashboard (Meilensteine b und g).
