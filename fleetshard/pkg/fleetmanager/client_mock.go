// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package fleetmanager

import (
	"github.com/stackrox/acs-fleet-manager/internal/dinosaur/pkg/api/private"
	"github.com/stackrox/acs-fleet-manager/internal/dinosaur/pkg/api/public"
	"sync"
)

// Ensure, that FleetManagerClientMock does implement FleetManagerClient.
// If this is not the case, regenerate this file with moq.
var _ FleetManagerClient = &FleetManagerClientMock{}

// FleetManagerClientMock is a mock implementation of FleetManagerClient.
//
// 	func TestSomethingThatUsesFleetManagerClient(t *testing.T) {
//
// 		// make and configure a mocked FleetManagerClient
// 		mockedFleetManagerClient := &FleetManagerClientMock{
// 			CreateCentralFunc: func(request public.CentralRequestPayload) (*public.CentralRequest, error) {
// 				panic("mock out the CreateCentral method")
// 			},
// 			DeleteCentralFunc: func(id string) error {
// 				panic("mock out the DeleteCentral method")
// 			},
// 			GetCentralFunc: func(id string) (*public.CentralRequest, error) {
// 				panic("mock out the GetCentral method")
// 			},
// 			GetManagedCentralListFunc: func() (*private.ManagedCentralList, error) {
// 				panic("mock out the GetManagedCentralList method")
// 			},
// 			UpdateStatusFunc: func(statuses map[string]private.DataPlaneCentralStatus) error {
// 				panic("mock out the UpdateStatus method")
// 			},
// 		}
//
// 		// use mockedFleetManagerClient in code that requires FleetManagerClient
// 		// and then make assertions.
//
// 	}
type FleetManagerClientMock struct {
	// CreateCentralFunc mocks the CreateCentral method.
	CreateCentralFunc func(request public.CentralRequestPayload) (*public.CentralRequest, error)

	// DeleteCentralFunc mocks the DeleteCentral method.
	DeleteCentralFunc func(id string) error

	// GetCentralFunc mocks the GetCentral method.
	GetCentralFunc func(id string) (*public.CentralRequest, error)

	// GetManagedCentralListFunc mocks the GetManagedCentralList method.
	GetManagedCentralListFunc func() (*private.ManagedCentralList, error)

	// UpdateStatusFunc mocks the UpdateStatus method.
	UpdateStatusFunc func(statuses map[string]private.DataPlaneCentralStatus) error

	// calls tracks calls to the methods.
	calls struct {
		// CreateCentral holds details about calls to the CreateCentral method.
		CreateCentral []struct {
			// Request is the request argument value.
			Request public.CentralRequestPayload
		}
		// DeleteCentral holds details about calls to the DeleteCentral method.
		DeleteCentral []struct {
			// ID is the id argument value.
			ID string
		}
		// GetCentral holds details about calls to the GetCentral method.
		GetCentral []struct {
			// ID is the id argument value.
			ID string
		}
		// GetManagedCentralList holds details about calls to the GetManagedCentralList method.
		GetManagedCentralList []struct {
		}
		// UpdateStatus holds details about calls to the UpdateStatus method.
		UpdateStatus []struct {
			// Statuses is the statuses argument value.
			Statuses map[string]private.DataPlaneCentralStatus
		}
	}
	lockCreateCentral         sync.RWMutex
	lockDeleteCentral         sync.RWMutex
	lockGetCentral            sync.RWMutex
	lockGetManagedCentralList sync.RWMutex
	lockUpdateStatus          sync.RWMutex
}

// CreateCentral calls CreateCentralFunc.
func (mock *FleetManagerClientMock) CreateCentral(request public.CentralRequestPayload) (*public.CentralRequest, error) {
	if mock.CreateCentralFunc == nil {
		panic("FleetManagerClientMock.CreateCentralFunc: method is nil but FleetManagerClient.CreateCentral was just called")
	}
	callInfo := struct {
		Request public.CentralRequestPayload
	}{
		Request: request,
	}
	mock.lockCreateCentral.Lock()
	mock.calls.CreateCentral = append(mock.calls.CreateCentral, callInfo)
	mock.lockCreateCentral.Unlock()
	return mock.CreateCentralFunc(request)
}

// CreateCentralCalls gets all the calls that were made to CreateCentral.
// Check the length with:
//     len(mockedFleetManagerClient.CreateCentralCalls())
func (mock *FleetManagerClientMock) CreateCentralCalls() []struct {
	Request public.CentralRequestPayload
} {
	var calls []struct {
		Request public.CentralRequestPayload
	}
	mock.lockCreateCentral.RLock()
	calls = mock.calls.CreateCentral
	mock.lockCreateCentral.RUnlock()
	return calls
}

// DeleteCentral calls DeleteCentralFunc.
func (mock *FleetManagerClientMock) DeleteCentral(id string) error {
	if mock.DeleteCentralFunc == nil {
		panic("FleetManagerClientMock.DeleteCentralFunc: method is nil but FleetManagerClient.DeleteCentral was just called")
	}
	callInfo := struct {
		ID string
	}{
		ID: id,
	}
	mock.lockDeleteCentral.Lock()
	mock.calls.DeleteCentral = append(mock.calls.DeleteCentral, callInfo)
	mock.lockDeleteCentral.Unlock()
	return mock.DeleteCentralFunc(id)
}

// DeleteCentralCalls gets all the calls that were made to DeleteCentral.
// Check the length with:
//     len(mockedFleetManagerClient.DeleteCentralCalls())
func (mock *FleetManagerClientMock) DeleteCentralCalls() []struct {
	ID string
} {
	var calls []struct {
		ID string
	}
	mock.lockDeleteCentral.RLock()
	calls = mock.calls.DeleteCentral
	mock.lockDeleteCentral.RUnlock()
	return calls
}

// GetCentral calls GetCentralFunc.
func (mock *FleetManagerClientMock) GetCentral(id string) (*public.CentralRequest, error) {
	if mock.GetCentralFunc == nil {
		panic("FleetManagerClientMock.GetCentralFunc: method is nil but FleetManagerClient.GetCentral was just called")
	}
	callInfo := struct {
		ID string
	}{
		ID: id,
	}
	mock.lockGetCentral.Lock()
	mock.calls.GetCentral = append(mock.calls.GetCentral, callInfo)
	mock.lockGetCentral.Unlock()
	return mock.GetCentralFunc(id)
}

// GetCentralCalls gets all the calls that were made to GetCentral.
// Check the length with:
//     len(mockedFleetManagerClient.GetCentralCalls())
func (mock *FleetManagerClientMock) GetCentralCalls() []struct {
	ID string
} {
	var calls []struct {
		ID string
	}
	mock.lockGetCentral.RLock()
	calls = mock.calls.GetCentral
	mock.lockGetCentral.RUnlock()
	return calls
}

// GetManagedCentralList calls GetManagedCentralListFunc.
func (mock *FleetManagerClientMock) GetManagedCentralList() (*private.ManagedCentralList, error) {
	if mock.GetManagedCentralListFunc == nil {
		panic("FleetManagerClientMock.GetManagedCentralListFunc: method is nil but FleetManagerClient.GetManagedCentralList was just called")
	}
	callInfo := struct {
	}{}
	mock.lockGetManagedCentralList.Lock()
	mock.calls.GetManagedCentralList = append(mock.calls.GetManagedCentralList, callInfo)
	mock.lockGetManagedCentralList.Unlock()
	return mock.GetManagedCentralListFunc()
}

// GetManagedCentralListCalls gets all the calls that were made to GetManagedCentralList.
// Check the length with:
//     len(mockedFleetManagerClient.GetManagedCentralListCalls())
func (mock *FleetManagerClientMock) GetManagedCentralListCalls() []struct {
} {
	var calls []struct {
	}
	mock.lockGetManagedCentralList.RLock()
	calls = mock.calls.GetManagedCentralList
	mock.lockGetManagedCentralList.RUnlock()
	return calls
}

// UpdateStatus calls UpdateStatusFunc.
func (mock *FleetManagerClientMock) UpdateStatus(statuses map[string]private.DataPlaneCentralStatus) error {
	if mock.UpdateStatusFunc == nil {
		panic("FleetManagerClientMock.UpdateStatusFunc: method is nil but FleetManagerClient.UpdateStatus was just called")
	}
	callInfo := struct {
		Statuses map[string]private.DataPlaneCentralStatus
	}{
		Statuses: statuses,
	}
	mock.lockUpdateStatus.Lock()
	mock.calls.UpdateStatus = append(mock.calls.UpdateStatus, callInfo)
	mock.lockUpdateStatus.Unlock()
	return mock.UpdateStatusFunc(statuses)
}

// UpdateStatusCalls gets all the calls that were made to UpdateStatus.
// Check the length with:
//     len(mockedFleetManagerClient.UpdateStatusCalls())
func (mock *FleetManagerClientMock) UpdateStatusCalls() []struct {
	Statuses map[string]private.DataPlaneCentralStatus
} {
	var calls []struct {
		Statuses map[string]private.DataPlaneCentralStatus
	}
	mock.lockUpdateStatus.RLock()
	calls = mock.calls.UpdateStatus
	mock.lockUpdateStatus.RUnlock()
	return calls
}
