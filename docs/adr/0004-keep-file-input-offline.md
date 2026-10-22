# Keep file input offline

CronJob definitions read from files are analyzed directly, without resolving them to live cluster objects or loading cluster credentials. This makes local validation reproducible and keeps the supplied configuration authoritative. Operators who want current cluster state use the live object-selection path instead.
