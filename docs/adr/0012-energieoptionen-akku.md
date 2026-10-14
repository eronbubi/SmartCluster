# ADR-0012: Energieoptionen und Akku-Hinweis

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Ein Laptop geht in den Standby, wenn man den Deckel schließt oder er im Leerlauf ist, und der Server ist dann weg. Alte Li-Ion-Akkus, die dauerhaft auf 100 % und warm gehalten werden, können sich aufblähen.

## Entscheidung
Der Wizard fragt nach der Zustimmung. Danach setzt der Agent mit dem Befehl `power.configure`:
- **Windows:** `powercfg` für das aktive Schema am Netzteil: Deckel → nichts tun, Standby und Ruhezustand aus.
- **Linux:** logind `HandleLidSwitch=ignore` und `HandleLidSwitchExternalPower=ignore`, Sleep-Targets maskiert.
- **Akku-Ladegrenze:** Unter Linux setzt der Agent `charge_control_end_threshold` auf 80 %, wo der Treiber das anbietet (nach Zustimmung). Unter Windows gibt es nur einen Hinweis auf das Hersteller-Tool, oder darauf, den Akku zu entfernen.

Die vorherigen Werte werden gespeichert und beim Entfernen des Nodes wiederhergestellt.

## Konsequenzen
- Das steht nicht in der ursprünglichen Anforderung, ist aber Voraussetzung für „idiotensicher“.
- Am Akku (Stromausfall) laufen Laptops weiter und leeren den Akku. Die Warnung „Node läuft auf Akku“ meldet das.
- Ändert der Nutzer die Werte nachträglich, zeigt der Hardware-Check `warn`.

## Aufgaben
1. [ ] `power.configure`, Prüfung im Hardware-Check, Wizard-Schritt (Meilenstein b).
