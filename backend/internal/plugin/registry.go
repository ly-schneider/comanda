package plugin

// All is the single source of truth for compiled-in plugins. It is empty until
// the first plugin is extracted; app wiring composes each entry's fx module and
// the router mounts each entry's routes.
var All = []Plugin{}
