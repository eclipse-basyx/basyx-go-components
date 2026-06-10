# ToDos

1. Update Secured example with new dynamic access rule api
2. update hostory example with new dynamic access rule api and access rule evidence
3. Check if multiple components in the same setup can have the new dynamic access rule api endpoints without conflicts (they access the same DB tables)
4. Check impact on DTR (maybe even leave it out? -> clarifiy with martin)

    heißt beim start würden wir von der json die tabellen neu erstellen für den "json ground truth" mode?

    also wir haben einmal eine env variable mit "json ground truth" und "enable /rules endpoints" rivhtig? und wir bnutzen nur die db um mit rules zu interagieren

5. Dynamically added openapi.yml routes for dynamic access rule api (and maybe also for /$history and /$recent-changes?)
6. Dokumentation for dynamic access rule api
