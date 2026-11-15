package api

import (
	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/config"

	otherrouter "github.com/gatework/motor/app/other/router"
)

// Start-up warnings about configurations that expose more than intended.

// reportGeneratorWriteRoutes says whether this process serves the code
// generator's writing endpoints, and to whom.
//
// The endpoints are gated on the mode, and the shipped configuration says dev -
// so the deployment most likely to be exposed is the one that changed nothing,
// and the one least likely to go looking. Silence there would leave the gate
// technically correct and practically useless.
//
// Nothing is said in demo mode. The routes are registered, but DemoEnv refuses
// all three as writes, so a warning would describe an exposure that is not there.
func reportGeneratorWriteRoutes() {
	if !generatorWriteRoutesNeedWarning() {
		return
	}
	log.NewHelper(log.Default()).Warnf("the code generator's writing endpoints are served in mode %q: "+
		"/api/v1/gen/{toproject,apitofile,todb} write Go and Vue source onto this host and rows "+
		"into this database, for the administrator and any role granted the code generator's "+
		"menus. Set application.mode to prod or test on anything that is not a workstation, "+
		"then restart: these routes were registered at start-up and a configuration reload does "+
		"not rebuild them.",
		config.Current().Settings.Application.Mode)
}

// generatorWriteRoutesNeedWarning reports whether there is an exposure to warn
// about: the endpoints are served, and nothing else is refusing them.
//
// Split from the logging so the decision can be tested. A warning nobody can
// make fire is indistinguishable from no warning at all, and this one exists
// precisely for the case nobody is looking at.
func generatorWriteRoutesNeedWarning() bool {
	return otherrouter.GenWriteRoutesEnabled() && config.Current().Settings.Application.Mode != "demo"
}
