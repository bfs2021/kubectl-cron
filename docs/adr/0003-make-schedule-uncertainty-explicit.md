# Make schedule uncertainty explicit

The Next occurrence is calculated from the current Schedule; it does not predict catch-up or guarantee Job execution. An unspecified timezone remains unknown unless the operator supplies a visibly labeled assumption, which never overrides a configured timezone. This avoids presenting the client machine's timezone or UTC as evidence of controller behavior while still allowing informed operators to calculate useful times.
