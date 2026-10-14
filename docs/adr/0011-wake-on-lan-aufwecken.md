# ADR-0011: Wake-on-LAN mit „Aufwecken“-Knopf

**Status:** Akzeptiert
**Datum:** 2026-10-14
**Entscheider:** Produktverantwortung

## Kontext
Der Hardware-Check prüft WoL. Ohne eine Funktion, die WoL auch nutzt, wäre der Check zwecklos.

## Entscheidung
Für Nodes, die offline sind, zeigt die Node-Detailseite einen Knopf **„Aufwecken“**.
- Die Kontrollzentrale schickt einen signierten Befehl `node.wake {mac}` an den Agent eines **online** Nodes im selben Subnetz, bevorzugt an den Node der Kontrollzentrale.
- Dieser Agent sendet das Magic Packet per UDP an den Broadcast des Subnetzes, Port 9.
- Der Knopf ist nur aktiv, wenn der letzte WoL-Check des Ziels `ok` oder `warn` war und ein passender Absender online ist.

Das Magic Packet geht vom Agent aus, nicht vom `control`-Container: Aus dem Docker-Bridge-Netz erreicht ein Broadcast das LAN nicht zuverlässig.

## Konsequenzen
- Neuer Befehlstyp `node.wake` (Meilenstein b).
- Die UI ist ehrlich: „Viele Laptops wachen nur aus dem Standby auf, nur am Netzteil und nur per LAN-Kabel.“
- Ein Erfolg wird erst bestätigt, wenn sich der Agent meldet. Timeout nach 3 min mit Hinweis.

## Aufgaben
1. [ ] `node.wake` im Agent, Knopf und Timeout-Logik (Meilenstein b).
