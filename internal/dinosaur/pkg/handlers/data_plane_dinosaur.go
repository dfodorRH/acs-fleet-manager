package handlers

import (
	"net/http"

	private "github.com/stackrox/acs-fleet-manager/generated/privateapi"
	"github.com/stackrox/acs-fleet-manager/internal/dinosaur/pkg/presenters"
	"github.com/stackrox/acs-fleet-manager/internal/dinosaur/pkg/services"
	"github.com/stackrox/acs-fleet-manager/pkg/handlers"

	"github.com/gorilla/mux"
	"github.com/stackrox/acs-fleet-manager/pkg/errors"
)

type dataPlaneDinosaurHandler struct {
	service         services.DataPlaneDinosaurService
	dinosaurService services.DinosaurService
	presenter       *presenters.ManagedCentralPresenter
}

// NewDataPlaneDinosaurHandler ...
func NewDataPlaneDinosaurHandler(service services.DataPlaneDinosaurService, dinosaurService services.DinosaurService, presenter *presenters.ManagedCentralPresenter) *dataPlaneDinosaurHandler {
	return &dataPlaneDinosaurHandler{
		service:         service,
		dinosaurService: dinosaurService,
		presenter:       presenter,
	}
}

// UpdateDinosaurStatuses ...
func (h *dataPlaneDinosaurHandler) UpdateDinosaurStatuses(w http.ResponseWriter, r *http.Request) {
	clusterID := mux.Vars(r)["id"]
	var data = map[string]private.CentralStatus{}

	cfg := &handlers.HandlerConfig{
		MarshalInto: &data,
		Validate:    []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			dataPlaneDinosaurStatus := presenters.ConvertDataPlaneDinosaurStatus(data)
			err := h.service.UpdateDataPlaneDinosaurService(ctx, clusterID, dataPlaneDinosaurStatus)
			return nil, err
		},
	}

	handlers.Handle(w, r, cfg, http.StatusOK)
}

// GetAll ...
func (h *dataPlaneDinosaurHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	clusterID := mux.Vars(r)["id"]
	cfg := &handlers.HandlerConfig{
		Validate: []handlers.Validate{
			handlers.ValidateLength(&clusterID, "id", &handlers.MinRequiredFieldLength, nil),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			centralRequests, err := h.dinosaurService.ListByClusterID(clusterID)
			if err != nil {
				return nil, err
			}

			managedCentralList := managedCentralList{
				Kind:  "ManagedCentralList",
				Items: []private.ManagedCentral{},
			}

			for i := range centralRequests {
				converted := h.presenter.PresentManagedCentral(centralRequests[i])
				managedCentralList.Items = append(managedCentralList.Items, converted)
			}
			return managedCentralList, nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}

type managedCentralList struct {
	Kind  string
	Items []private.ManagedCentral
}
