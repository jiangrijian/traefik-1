package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	"github.com/containous/traefik/v2/pkg/config/runtime"
	"github.com/containous/traefik/v2/pkg/config/static"
	"github.com/containous/traefik/v2/pkg/log"
	"github.com/containous/traefik/v2/pkg/version"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/mux"
)

type apiError struct {
	Message string `json:"message"`
}

func writeError(rw http.ResponseWriter, msg string, code int) {
	data, err := json.Marshal(apiError{Message: msg})
	if err != nil {
		http.Error(rw, msg, code)
		return
	}

	http.Error(rw, string(data), code)
}

type serviceInfoRepresentation struct {
	*runtime.ServiceInfo
	ServerStatus map[string]string `json:"serverStatus,omitempty"`
}

// RunTimeRepresentation is the configuration information exposed by the API handler.
type RunTimeRepresentation struct {
	Routers     map[string]*runtime.RouterInfo        `json:"routers,omitempty"`
	Middlewares map[string]*runtime.MiddlewareInfo    `json:"middlewares,omitempty"`
	Services    map[string]*serviceInfoRepresentation `json:"services,omitempty"`
	TCPRouters  map[string]*runtime.TCPRouterInfo     `json:"tcpRouters,omitempty"`
	TCPServices map[string]*runtime.TCPServiceInfo    `json:"tcpServices,omitempty"`
}

// Handler serves the configuration and status of Traefik on API endpoints.
type Handler struct {
	dashboard bool
	debug     bool
	// runtimeConfiguration is the data set used to create all the data representations exposed by the API.
	runtimeConfiguration *runtime.Configuration
	staticConfig         static.Configuration
	// statistics           *types.Statistics
	// stats                *thoasstats.Stats // FIXME stats
	// StatsRecorder         *middlewares.StatsRecorder // FIXME stats
	dashboardAssets *assetfs.AssetFS
}

// NewBuilder returns a http.Handler builder based on runtime.Configuration
func NewBuilder(staticConfig static.Configuration) func(*runtime.Configuration) http.Handler {
	return func(configuration *runtime.Configuration) http.Handler {
		router := mux.NewRouter()
		New(staticConfig, configuration).Append(router)
		return router
	}
}

// New returns a Handler defined by staticConfig, and if provided, by runtimeConfig.
// It finishes populating the information provided in the runtimeConfig.
func New(staticConfig static.Configuration, runtimeConfig *runtime.Configuration) *Handler {
	rConfig := runtimeConfig
	if rConfig == nil {
		rConfig = &runtime.Configuration{}
	}

	return &Handler{
		dashboard: staticConfig.API.Dashboard,
		// statistics:           staticConfig.API.Statistics,
		dashboardAssets:      staticConfig.API.DashboardAssets,
		runtimeConfiguration: rConfig,
		staticConfig:         staticConfig,
		debug:                staticConfig.API.Debug,
	}
}

// Append add api routes on a router
func (h Handler) Append(router *mux.Router) {
	if h.debug {
		DebugHandler{}.Append(router)
	}

	router.Methods(http.MethodGet).Path("/api/rawdata").HandlerFunc(h.getRuntimeConfiguration)

	// Experimental endpoint
	router.Methods(http.MethodGet).Path("/api/overview").HandlerFunc(h.getOverview)

	router.Methods(http.MethodGet).Path("/api/entrypoints").HandlerFunc(h.getEntryPoints)
	router.Methods(http.MethodGet).Path("/api/entrypoints/{entryPointID}").HandlerFunc(h.getEntryPoint)

	router.Methods(http.MethodGet).Path("/api/http/routers").HandlerFunc(h.getRouters)
	router.Methods(http.MethodGet).Path("/api/http/routers/{routerID}").HandlerFunc(h.getRouter)
	router.Methods(http.MethodGet).Path("/api/http/services").HandlerFunc(h.getServices)
	router.Methods(http.MethodGet).Path("/api/http/services/{serviceID}").HandlerFunc(h.getService)
	router.Methods(http.MethodGet).Path("/api/http/middlewares").HandlerFunc(h.getMiddlewares)
	router.Methods(http.MethodGet).Path("/api/http/middlewares/{middlewareID}").HandlerFunc(h.getMiddleware)

	router.Methods(http.MethodGet).Path("/api/tcp/routers").HandlerFunc(h.getTCPRouters)
	router.Methods(http.MethodGet).Path("/api/tcp/routers/{routerID}").HandlerFunc(h.getTCPRouter)
	router.Methods(http.MethodGet).Path("/api/tcp/services").HandlerFunc(h.getTCPServices)
	router.Methods(http.MethodGet).Path("/api/tcp/services/{serviceID}").HandlerFunc(h.getTCPService)

	// FIXME stats
	// health route
	// router.Methods(http.MethodGet).Path("/health").HandlerFunc(p.getHealthHandler)

	version.Handler{}.Append(router)

	if h.dashboard {
		DashboardHandler{Assets: h.dashboardAssets}.Append(router)
	}
}

func (h Handler) getRuntimeConfiguration(rw http.ResponseWriter, request *http.Request) {
	siRepr := make(map[string]*serviceInfoRepresentation, len(h.runtimeConfiguration.Services))
	for k, v := range h.runtimeConfiguration.Services {
		siRepr[k] = &serviceInfoRepresentation{
			ServiceInfo:  v,
			ServerStatus: v.GetAllStatus(),
		}
	}

	result := RunTimeRepresentation{
		Routers:     h.runtimeConfiguration.Routers,
		Middlewares: h.runtimeConfiguration.Middlewares,
		Services:    siRepr,
		TCPRouters:  h.runtimeConfiguration.TCPRouters,
		TCPServices: h.runtimeConfiguration.TCPServices,
	}

	rw.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(rw).Encode(result)
	if err != nil {
		log.FromContext(request.Context()).Error(err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}
}

func getProviderName(id string) string {
	return strings.SplitN(id, "@", 2)[1]
}

func extractType(element interface{}) string {
	v := reflect.ValueOf(element).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Kind() == reflect.Ptr && field.Elem().Kind() == reflect.Struct {
			if !field.IsNil() {
				return v.Type().Field(i).Name
			}
		}
	}
	return ""
}
