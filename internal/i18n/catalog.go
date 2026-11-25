package i18n

type entry struct{ en, fr string }

// catalog holds every translated message. Both languages take the same fmt verbs
// in the same order (checked by the tests).
var catalog = map[string]entry{
	// ── API errors ───────────────────────────────────────────────────────────────
	"api.route_unknown":  {"unknown route", "route inconnue"},
	"api.read_only":      {"read-only", "lecture seule"},
	"api.period_invalid": {"unknown period: window=1h, 24h or 7d, or from and to (Unix seconds)", "période inconnue : window=1h, 24h ou 7d, ou from et to (secondes Unix)"},
	"api.range_invalid":  {"empty range, or outside the last 7 days", "intervalle vide ou hors des 7 derniers jours"},
	"api.ip_invalid":     {"invalid IP address", "adresse IP invalide"},
	"api.ip_no_requests": {"no request from this IP over the period", "aucune requête de cette IP sur la période"},
	"api.host_refused":   {"host not allowed: open the page at its usual address", "hôte non autorisé : ouvrir la page par son adresse habituelle"},
	"api.header_missing": {"%s header missing", "en-tête %s absent"},
	"api.cross_site":     {"cross-site request refused", "requête d'un autre site refusée"},
	"api.origin_refused": {"origin refused", "origine refusée"},
	"api.block_disabled": {"blocking disabled: no CrowdSec machine account configured", "blocage désactivé : aucun compte machine CrowdSec configuré"},
	"api.ban_body":       {`JSON body expected: {"duration": "24h", "reason": "…"}`, `corps JSON attendu : {"duration": "24h", "reason": "…"}`},
	"api.ban_duration":   {"unknown duration: 4h, 24h, 7d or 30d", "durée inconnue : 4h, 24h, 7d ou 30d"},

	// ── Page warnings ────────────────────────────────────────────────────────────
	"warn.crowdsec_unreachable": {"CrowdSec is unreachable: current bans are not shown.", "CrowdSec injoignable : les bans en cours ne sont pas affichés."},
	"warn.no_lines":             {"No line read from access.log: nothing is being monitored.", "Aucune ligne lue dans access.log : rien n'est surveillé."},
	"warn.stale":                {"Data is late: the last line read is %d min old. The green below proves nothing.", "Données en retard : la dernière ligne lue date d'il y a %d min. Le vert ci-dessous ne prouve rien."},
	"warn.capped":               {"Exceptional traffic: the memory cap was reached, so some IPs and URLs are grouped under “others”. Totals remain exact.", "Trafic exceptionnel : le plafond mémoire est atteint, une partie des IP et des URL est regroupée sous « autres ». Les totaux restent exacts."},
	"warn.geo":                  {"GeoLite2 databases unreadable: countries and operators are not shown, ASN rules do not apply, and alerts are suspended to avoid false alarms.", "Bases GeoLite2 illisibles : pays et opérateurs ne sont pas affichés, les règles par ASN ne s'appliquent pas, et les alertes sont suspendues pour ne pas alerter à tort."},
	"warn.telegram_off":         {"Telegram is not configured (TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID): no alert is sent.", "Telegram n'est pas configuré (TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID) : aucune alerte ne part."},
	"warn.names_invalid_kept":   {"names.yaml is invalid: the previous rules still apply (%v).", "names.yaml est invalide : les règles précédentes restent appliquées (%v)."},
	"warn.names_invalid_none":   {"names.yaml is invalid: no IP is named, and success alerts are suspended (%v).", "names.yaml est invalide : aucune IP n'est nommée, et les alertes de succès sont suspendues (%v)."},
	"warn.lists":                {"Trust lists: %s.", "Listes de confiance : %s."},
	"warn.backfill":             {"Incomplete backfill of the archives: %s", "Relecture incomplète des archives : %s"},

	// ── Trust lists ──────────────────────────────────────────────────────────────
	"lists.crowdsec_unset":      {"the “crowdsec” list is used in names.yaml but CROWDSEC_WHITELIST is not set: it cannot be checked", "la liste « crowdsec » est utilisée dans names.yaml mais CROWDSEC_WHITELIST n'est pas défini : contrôle impossible"},
	"lists.crowdsec_unreadable": {"CrowdSec whitelist (%s) unreadable or empty: the “crowdsec” list cannot be checked", "whitelist CrowdSec (%s) illisible ou vide : contrôle de la liste « crowdsec » impossible"},
	"lists.traefik_unset":       {"the “%s” list is used in names.yaml but TRAEFIK_DYNAMIC is not set: it cannot be checked", "la liste « %s » est utilisée dans names.yaml mais TRAEFIK_DYNAMIC n'est pas défini : contrôle impossible"},
	"lists.traefik_unreadable":  {"Traefik dynamic configuration (%s) unreadable: the “%s” list cannot be checked", "fichier dynamique de Traefik (%s) illisible : contrôle de la liste « %s » impossible"},
	"lists.middleware_missing":  {"ipAllowList middleware “%s” missing or empty in %s: it cannot be checked", "middleware ipAllowList « %s » introuvable ou vide dans %s : contrôle impossible"},
	"lists.invalid_entry":       {"%s: invalid entry “%s”", "%s : entrée « %s » invalide"},
	"lists.missing_in_file":     {"%s is in names.yaml (%s) but not in %s", "%s est dans names.yaml (%s) mais pas dans %s"},
	"lists.missing_in_names":    {"%s is in %s but not in names.yaml (%s)", "%s est dans %s mais pas dans names.yaml (%s)"},

	// ── Ban refusals ─────────────────────────────────────────────────────────────
	"ban.private":          {"Private or reserved address: a ban would be useless and could cut off the local network.", "Adresse privée ou réservée : un ban ne servirait à rien et pourrait couper le réseau local."},
	"ban.named":            {"IP named “%s” in names.yaml: remove the rule first to be able to ban it.", "IP nommée « %s » dans names.yaml : retirer d'abord la règle pour pouvoir la bannir."},
	"ban.names_not_loaded": {"names.yaml is not loaded: cannot check that this IP is not trusted.", "names.yaml n'est pas chargé : impossible de vérifier que cette IP n'est pas de confiance."},

	// ── Verdicts (notifications) ─────────────────────────────────────────────────
	"verdict.known":   {"known", "connue"},
	"verdict.success": {"success", "succès"},
	"verdict.public":  {"public", "public"},
	"verdict.blocked": {"blocked", "bloquée"},
	"verdict.probing": {"probing", "tâtonne"},
	"verdict.other":   {"other", "autre"},

	// ── Live alert ───────────────────────────────────────────────────────────────
	"tg.open":          {"Open the dashboard", "Ouvrir le tableau de bord"},
	"tg.whois_unknown": {"unknown country and operator", "pays et opérateur inconnus"},
	"tg.success_title": {"🔴 <b>Unknown IP got a success</b>", "🔴 <b>Succès d'une IP inconnue</b>"},
	"tg.looks_like":    {"Looks like: %s (outside the routers or paths of its rule)", "Ressemble à : %s (hors des routers ou chemins de sa règle)"},
	"tg.sensitive":     {"⚠️ Path probed by scanners.", "⚠️ Chemin recherché par les scanners."},
	"tg.name_it":       {"If this is legitimate, name this IP in <code>names.yaml</code>.", "Si c'est légitime, nommer cette IP dans <code>names.yaml</code>."},

	// ── Daily summary ────────────────────────────────────────────────────────────
	"daily.title":            {"📊 <b>Traefik — last 24 h</b>", "📊 <b>Traefik — dernières 24 h</b>"},
	"daily.red.one":          {"🔴 <b>%d unknown IP got a success</b>", "🔴 <b>%d IP inconnue a obtenu un succès</b>"},
	"daily.red.other":        {"🔴 <b>%d unknown IPs got a success</b>", "🔴 <b>%d IP inconnues ont obtenu un succès</b>"},
	"daily.more":             {"  … and %d more", "  … et %d autre(s)"},
	"daily.offender.one":     {"  • <code>%s</code> %s: %d success%s", "  • <code>%s</code> %s : %d succès%s"},
	"daily.offender.other":   {"  • <code>%s</code> %s: %d successes%s", "  • <code>%s</code> %s : %d succès%s"},
	"daily.incomplete":       {"Incomplete log coverage: the verdict may be incomplete.", "Couverture des journaux incomplète : le verdict peut être partiel."},
	"warn.sensitive_partial": {"Some sensitive-path hits in this period were evicted. Counts may be low.", "Certains accès aux chemins sensibles de cette période ont été évincés. Les totaux peuvent être sous-estimés."},
	"daily.green":            {"🟢 No unknown IP got a success", "🟢 Aucune IP inconnue n'a obtenu de succès"},
	"daily.requests":         {"Requests: %s (avg %s req/s, peak %d)", "Requêtes : %s (moy. %s req/s, pic %d)"},
	"daily.aborted":          {"aborted", "abandons"},
	"daily.statuses":         {"Statuses: %s", "Statuts : %s"},
	"daily.volume":           {"Volume: %s in, %s out", "Volume : %s entrants, %s sortants"},
	"daily.ips":              {"IPs: %d named, %d unnamed", "IP : %d nommées, %d non nommées"},
	"daily.5xx_title":        {"⚠️ <b>5xx by router</b>", "⚠️ <b>5xx par router</b>"},
	"daily.5xx_line":         {"  • %s: %d", "  • %s : %d"},
	"daily.no5xx":            {"✅ No 5xx error", "✅ Aucune erreur 5xx"},
	"daily.top":              {"🌍 <b>Top unnamed IPs</b>", "🌍 <b>Top IP non nommées</b>"},
	"daily.banned":           {", banned (%s)", ", bannie (%s)"},
	"daily.ip_line":          {"  • <code>%s</code>%s: %s req, %s%s", "  • <code>%s</code>%s : %s req, %s%s"},
	"daily.sensitive":        {"🚨 <b>Sensitive paths served (2xx)</b>", "🚨 <b>Chemins sensibles servis (2xx)</b>"},
	"daily.cs_unreachable":   {"🛡️ CrowdSec unreachable: current bans unknown", "🛡️ CrowdSec injoignable : bans en cours inconnus"},
	"daily.cs_bans.one":      {"🛡️ CrowdSec: %d current ban", "🛡️ CrowdSec : %d ban en cours"},
	"daily.cs_bans.other":    {"🛡️ CrowdSec: %d current bans", "🛡️ CrowdSec : %d bans en cours"},
}
