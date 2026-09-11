# ABAC and ReBAC Coexistence: Nicht-Interferenznachweis

> [!WARNING]
> ReBAC ist eine experimentelle BaSyx-Erweiterung. APIs und Semantik können sich inkompatibel ändern. ABAC und der Modus `legacy-abac` bleiben davon unabhängig.

> Kurzfassung für Architektur- und Security-Reviews; ausgelegt auf maximal zwei A4-Seiten.

## Garantie und Grenze des Nachweises

Der Modus `legacy-abac` verwendet weiterhin den unveränderten ABAC-Setup- und Evaluationspfad. Im Modus `resource-bound-first` wird dasselbe ABAC-Modell als unabhängiger Fallback ausgewertet. Eine fehlende ReBAC-Erlaubnis kann ein ABAC-`ALLOW` nicht widerrufen:

```text
Konkrete Ressource: ALLOW = OWNER OR ReBAC(resource) OR ABAC(object/resource)
Collection-Aufruf:   ALLOW = ABAC(route)
Collection-Ergebnis: sichtbar = OWNER OR ReBAC(resource) OR ABAC(object/resource)
```

„ABAC unverändert“ bedeutet hier zweierlei:

1. Parser, Materialisierung, Methoden-/Rechte-Mapping, Objektauflösung, Formelvereinfachung und der bisherige ABAC-Middleware-Setup sind gegenüber `origin/main` quelltextidentisch.
2. Der ReBAC-Fallback ruft den ABAC-Evaluator mit demselben `EvalInput` und denselben Optionen auf und übernimmt dessen vollständiges `AuthorizationEvaluation` unverändert.

Die neue Modusauswahl sowie gemeinsame SQL-Filteradapter sind Integrationscode und daher erwartungsgemäß neu beziehungsweise erweitert. Sie ändern nicht den ABAC-Evaluator. Technische ReBAC-/Datenbankfehler bleiben fail-closed; das ist kein ReBAC-Deny, das ein ABAC-Allow überstimmt.

## Quelltextbeweis

Die unveränderten ABAC-Produktionsdateien sind:

- `abac_engine.go`, `abac_engine_attributes.go`, `abac_engine_materialization.go`, `abac_engine_methods.go`, `abac_engine_objects.go`
- `input_eval.go`, `authorize.go`
- `abacpolicy/setup.go`

Der folgende reproduzierbare Vergleich endet mit Exit-Code `0` und ohne Ausgabe:

```bash
git diff --exit-code origin/main -- \
  internal/common/security/abac_engine.go \
  internal/common/security/abac_engine_attributes.go \
  internal/common/security/abac_engine_materialization.go \
  internal/common/security/abac_engine_methods.go \
  internal/common/security/abac_engine_objects.go \
  internal/common/security/input_eval.go \
  internal/common/security/authorize.go \
  internal/common/security/abacpolicy/setup.go
```

Die Umschaltung befindet sich stattdessen in der neuen Datei [`configured_setup.go`](../../internal/common/security/abacpolicy/configured_setup.go): Ohne `resource-bound-first` delegiert sie unmittelbar an `SetupSecurityWithABACRepository`; mit ReBAC installiert sie den ReBAC-Adapter und übergibt das aktive ABAC-Repository als Fallback.

Der direkte Fallback ist in [`resource_bound_query.go`](../../internal/common/security/resource_bound_query.go#L71) sichtbar: `authorizeABACFallback` besteht ausschließlich aus dem Aufruf von `model.AuthorizeWithFilterWithOptions(input, opts)`. ReBAC-Policies werden in einem getrennten Pfad ausgewertet. Bei Collections werden lediglich bestehende ABAC-Regeln nach `ROUTE` für die Aufrufzulassung und nach Ressourcenobjekten für die Ergebnisfilterung getrennt; beide Teilmengen verwenden weiterhin den unveränderten Evaluator.

Beim Zusammenführen beginnt der SQL-Ausdruck mit Ownership, ergänzt passende effektive ReBAC-Policies und fügt anschließend das ABAC-Ergebnis als weitere OR-Alternative hinzu ([`resource_bound_query.go`](../../internal/common/security/resource_bound_query.go#L306)). Es gibt keinen ReBAC-Deny-Zweig, der das ABAC-Ergebnis negiert.

## Automatisierter Verhaltensbeweis

- `TestSetupConfiguredSecurityLegacyModeUsesUnchangedABACSetup` beweist, dass die neue Modusauswahl ohne `resource-bound-first` das bisherige ABAC-Repository lädt und dessen Setup verwendet ([Test](../../internal/common/security/abacpolicy/configured_setup_test.go#L35)).
- `TestResourceBoundFallbackPreservesABACEvaluation` führt identische READ-/UPDATE-Anfragen als Allow und No-Match einmal direkt durch den Legacy-Evaluator und einmal durch den ReBAC-Fallback. Verglichen wird die vollständige Struktur einschließlich `Allowed`, Reason, Policy-/Rule-ID und QueryFilter mit `require.Equal` ([Test](../../internal/common/security/resource_bound_model_test.go#L159)).
- `TestResourceBoundAndABACPermissionsAreCombined` installiert absichtlich eine nicht passende beziehungsweise stärker filternde ReBAC-Policy. Das vorhandene ABAC-Allow liefert trotzdem die unveränderte Ressource mit HTTP `200` ([Test](../../internal/common/security/integration_tests/resource_bound_integration_test.go#L330)).
- `TestAdminCreatedAASIsHiddenWithoutReBACOrABACRead` beweist die OR-Semantik beim direkten GET und beim gefilterten GetAll: Owner sieht die eigene AAS, ein objektbezogen per ABAC berechtigter Auditor sieht alle passenden AAS ([Test](../../internal/common/security/integration_tests/resource_bound_integration_test.go#L350)).
- Die bestehenden ABAC-Unit- und Repository-Tests werden unverändert ausgeführt. Damit bleibt `legacy-abac` durch die bisherige Testsuite abgedeckt, während die ReBAC-Integration dieselben ABAC-Ergebnisse zusätzlich im Hybridmodus prüft.

## Praktische Bedeutung

- **Ohne ReBAC:** Konfiguration, Regeln, Management-API, Claim-Auswertung, Rechte-Mapping und Filter verhalten sich wie auf `origin/main`.
- **Mit ReBAC:** ABAC entscheidet weiterhin unabhängig. ReBAC und Ownership können zusätzliche konkrete Ressourcen freigeben, aber kein ABAC-Allow entfernen.
- **Collections:** CREATE sowie die Zulassung zu GetAll bleiben ABAC-Aufgaben. ReBAC wirkt ausschließlich auf konkrete Ressourcen und auf die Sichtbarkeit einzelner Ergebnisse.

Damit ist Nicht-Interferenz sowohl statisch (quelltextidentischer ABAC-Kern) als auch dynamisch (vollständiger Ergebnisvergleich und datenbankgestützte Hybridtests) nachgewiesen.
