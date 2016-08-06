//
//Copyright [2016] [SnapRoute Inc]
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//       Unless required by applicable law or agreed to in writing, software
//       distributed under the License is distributed on an "AS IS" BASIS,
//       WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//       See the License for the specific language governing permissions and
//       limitations under the License.
//
// _______  __       __________   ___      _______.____    __    ____  __  .___________.  ______  __    __
// |   ____||  |     |   ____\  \ /  /     /       |\   \  /  \  /   / |  | |           | /      ||  |  |  |
// |  |__   |  |     |  |__   \  V  /     |   (----` \   \/    \/   /  |  | `---|  |----`|  ,----'|  |__|  |
// |   __|  |  |     |   __|   >   <       \   \      \            /   |  |     |  |     |  |     |   __   |
// |  |     |  `----.|  |____ /  .  \  .----)   |      \    /\    /    |  |     |  |     |  `----.|  |  |  |
// |__|     |_______||_______/__/ \__\ |_______/        \__/  \__/     |__|     |__|      \______||__|  |__|
//

package faultMgr

import (
	"errors"
	"fmt"
	"infra/fMgrd/objects"
	"models/events"
	"utils/eventUtils"
)

func generateFaultObjKey(ownerName string, srcObjName string, srcObjKey interface{}) FaultObjKey {
	keyMap, exist := events.EventKeyMap[ownerName]
	if !exist {
		return ""
	}
	obj, exist := keyMap[srcObjName]
	if !exist {
		return ""
	}
	obj = srcObjKey
	return FaultObjKey(fmt.Sprintf("%v %v", srcObjName, obj))
}

func (fMgr *FaultManager) GetBulkFaultState(fromIdx int, count int) (*objects.FaultStateGetInfo, error) {
	var retObj objects.FaultStateGetInfo

	fMgr.FRBRWMutex.RLock()
	faults := fMgr.FaultRB.GetListOfEntriesFromRingBuffer()
	fMgr.FRBRWMutex.RUnlock()
	length := len(faults)
	fState := make([]objects.FaultState, count)

	var i int
	var j int

	for i, j = 0, fromIdx; i < count && j < length; j++ {
		fIntf := faults[length-j-1]
		fault := fIntf.(FaultRBEntry)
		var fObj objects.FaultState
		fObj.OwnerId = int32(fault.OwnerId)
		fObj.EventId = int32(fault.EventId)
		evtKey := EventKey{
			DaemonId: fault.OwnerId,
			EventId:  fault.EventId,
		}
		fEnt, exist := fMgr.FaultEventMap[evtKey]
		if !exist {
			j--
			continue
		}
		fObj.OwnerName = fEnt.FaultOwnerName
		fObj.EventName = fEnt.FaultEventName
		fObj.SrcObjName = fEnt.FaultSrcObjName
		fObj.Description = fault.Description
		fObj.OccuranceTime = fault.OccuranceTime.String()
		fObj.SrcObjKey = fmt.Sprintf("%v", fault.SrcObjKey)
		if fault.Resolved == true {
			fObj.ResolutionTime = fault.ResolutionTime.String()
		} else {
			fObj.ResolutionTime = "N/A"
		}
		fState[i] = fObj
		i++
	}
	retObj.EndIdx = j
	retObj.Count = i
	if j != length {
		retObj.More = true
	}
	retObj.List = fState
	return &retObj, nil
}

func (fMgr *FaultManager) processEvents(evt eventUtils.Event) error {
	evtKey := EventKey{
		DaemonId: int(evt.OwnerId),
		EventId:  int(evt.EvtId),
	}

	if fEnt, exist := fMgr.FaultEventMap[evtKey]; exist {
		if fEnt.RaiseFault == false {
			return nil
		}
		err := fMgr.ProcessFaultyEvents(evt)
		fMgr.logger.Debug(fmt.Sprintln("Fault Database:", fMgr.FaultMap))
		fMgr.logger.Debug(fmt.Sprintln("Alarm Database:", fMgr.AlarmMap))
		fMgr.logger.Debug(fmt.Sprintln("Fault Ring Buffer:", fMgr.FaultRB.GetListOfEntriesFromRingBuffer()))
		fMgr.logger.Debug(fmt.Sprintln("Alarm Ring Buffer:", fMgr.AlarmRB.GetListOfEntriesFromRingBuffer()))
		return err
	}
	if ent, exist := fMgr.NonFaultEventMap[evtKey]; exist {
		if ent.IsClearingEvent == true {
			err := fMgr.ProcessFaultClearingEvents(evt)
			fMgr.logger.Debug(fmt.Sprintln("Fault Database:", fMgr.FaultMap))
			fMgr.logger.Debug(fmt.Sprintln("Alarm Database:", fMgr.AlarmMap))
			fMgr.logger.Debug(fmt.Sprintln("Fault Ring Buffer:", fMgr.FaultRB.GetListOfEntriesFromRingBuffer()))
			fMgr.logger.Debug(fmt.Sprintln("Alarm Ring Buffer:", fMgr.AlarmRB.GetListOfEntriesFromRingBuffer()))
			return err
		}
		return nil
	}

	return errors.New("Unrecognized Event for Fault Manager")
}

func (fMgr *FaultManager) ProcessFaultyEvents(evt eventUtils.Event) error {
	fMgr.logger.Debug(fmt.Sprintln("Processing Faulty Events:", evt))
	return fMgr.CreateEntryInFaultAlarmDB(evt)
}

func (fMgr *FaultManager) ProcessFaultClearingEvents(evt eventUtils.Event) error {
	fMgr.logger.Debug(fmt.Sprintln("Processing Faulty Events:", evt))
	return fMgr.DeleteEntryFromFaultAlarmDB(evt)
}

func (fMgr *FaultManager) AddFaultEntryInRB(evt eventUtils.Event) int {
	fRBEnt := FaultRBEntry{
		OwnerId:        int(evt.OwnerId),
		EventId:        int(evt.EvtId),
		OccuranceTime:  evt.TimeStamp,
		FaultSeqNumber: fMgr.FaultSeqNumber,
		SrcObjKey:      evt.SrcObjKey,
		Description:    evt.Description,
	}

	fMgr.FRBRWMutex.Lock()
	idx := fMgr.FaultRB.InsertIntoRingBuffer(fRBEnt)
	fMgr.FRBRWMutex.Unlock()
	return idx
}

func (fMgr *FaultManager) CreateEntryInFaultAlarmDB(evt eventUtils.Event) error {
	evtKey := EventKey{
		DaemonId: int(evt.OwnerId),
		EventId:  int(evt.EvtId),
	}

	fMgr.FMapRWMutex.Lock()
	if fMgr.FaultMap[evtKey] == nil {
		fMgr.FaultMap[evtKey] = make(map[FaultObjKey]FaultData)
	}

	fDataMapEnt, _ := fMgr.FaultMap[evtKey]
	fObjKey := generateFaultObjKey(evt.OwnerName, evt.SrcObjName, evt.SrcObjKey)
	if fObjKey == "" {
		fMgr.FMapRWMutex.Unlock()
		return errors.New("Error generating fault object key")
	}

	fDataEnt, exist := fDataMapEnt[fObjKey]
	if exist {
		fMgr.FMapRWMutex.Unlock()
		fMgr.logger.Info("Already have corresponding fault in fault database")
		return nil
	}

	faultIdx := fMgr.AddFaultEntryInRB(evt)
	if faultIdx == -1 {
		fMgr.FMapRWMutex.Unlock()
		return errors.New("Unable to add entry in fault database")
	}

	fDataEnt.FaultListIdx = faultIdx
	fDataEnt.FaultSeqNumber = fMgr.FaultSeqNumber
	fMgr.FaultSeqNumber++

	//if alarm doen't exist for given Fault Start Alarm Timer
	// else stop the Alarm Removing Timer
	fMgr.AMapRWMutex.Lock()
	aDataMapEnt, exist := fMgr.AlarmMap[evtKey]
	if exist == false {
		fDataEnt.CreateAlarmTimer = fMgr.StartAlarmTimer(evt)
	} else {
		aDataEnt, exist := aDataMapEnt[fObjKey]
		if !exist {
			fDataEnt.CreateAlarmTimer = fMgr.StartAlarmTimer(evt)
		} else {
			ret := aDataEnt.RemoveAlarmTimer.Stop()
			if ret == true {
				fMgr.logger.Debug("Alarm corresponding to event cannot be removed as we received a fault again")
				aDataMapEnt[fObjKey] = aDataEnt
				fMgr.AlarmMap[evtKey] = aDataMapEnt
			} else {
				fMgr.logger.Debug("Either alarm removal timer doesnot exist or it cannot be stopped")
			}
		}
	}
	fMgr.AMapRWMutex.Unlock()
	fDataMapEnt[fObjKey] = fDataEnt
	fMgr.FaultMap[evtKey] = fDataMapEnt
	fMgr.FMapRWMutex.Unlock()
	return nil
}

func (fMgr *FaultManager) DeleteEntryFromFaultAlarmDB(evt eventUtils.Event) error {
	evtKey := EventKey{
		DaemonId: int(evt.OwnerId),
		EventId:  int(evt.EvtId),
	}

	cFEnt, exist := fMgr.NonFaultEventMap[evtKey]
	if !exist {
		return errors.New("Error finding the fault for fault clearing event")
	}
	fEvtKey := EventKey{
		DaemonId: cFEnt.FaultOwnerId,
		EventId:  cFEnt.FaultEventId,
	}

	if fEnt, exist := fMgr.FaultEventMap[fEvtKey]; exist {
		if fEnt.RaiseFault == false {
			return nil
		}
	} else {
		return errors.New("Unbale to find the corresponding fault Event")
	}
	fObjKey := generateFaultObjKey(evt.OwnerName, evt.SrcObjName, evt.SrcObjKey)
	if fObjKey == "" {
		return errors.New("Error generating fault object key")
	}

	fMgr.FMapRWMutex.Lock()
	fDataMapEnt, exist := fMgr.FaultMap[fEvtKey]
	if !exist {
		fMgr.logger.Debug(fmt.Sprintln("No such fault occured to be cleared, no entry faound in fault database", evt))
		fMgr.FMapRWMutex.Unlock()
		return nil
	}

	fDataEnt, exist := fDataMapEnt[fObjKey]
	if !exist {
		fMgr.logger.Debug(fmt.Sprintln("No such fault occured to be cleared, no entry faound in fault data", evt))
		fMgr.FMapRWMutex.Unlock()
		return nil
	}
	fMgr.FRBRWMutex.RLock()
	fIntf := fMgr.FaultRB.GetEntryFromRingBuffer(fDataEnt.FaultListIdx)
	fMgr.FRBRWMutex.RUnlock()
	fDBKey := fIntf.(FaultRBEntry)
	if fDataEnt.FaultSeqNumber == fDBKey.FaultSeqNumber {
		fDBKey.ResolutionTime = evt.TimeStamp
		fDBKey.Resolved = true
		fMgr.FRBRWMutex.Lock()
		fMgr.FaultRB.UpdateEntryInRingBuffer(fDBKey, fDataEnt.FaultListIdx)
		fMgr.FRBRWMutex.Unlock()
		fMgr.AMapRWMutex.Lock()
		aDataMapEnt, exist := fMgr.AlarmMap[fEvtKey]
		if !exist {
			ret := fDataEnt.CreateAlarmTimer.Stop()
			if ret == true {
				fMgr.logger.Debug(fmt.Sprintln("Alarm timer is stopped for", evt))
			}
		} else {
			aDataEnt, exist := aDataMapEnt[fObjKey]
			if !exist {
				ret := fDataEnt.CreateAlarmTimer.Stop()
				if ret == true {
					fMgr.logger.Debug(fmt.Sprintln("Alarm timer is stopped for", evt))
				}
			} else {
				aDataEnt.RemoveAlarmTimer = fMgr.StartAlarmRemoveTimer(evt)
				aDataMapEnt[fObjKey] = aDataEnt
				fMgr.AlarmMap[fEvtKey] = aDataMapEnt
			}
		}
		fMgr.AMapRWMutex.Unlock()
		delete(fDataMapEnt, fObjKey)
		fMgr.FaultMap[fEvtKey] = fDataMapEnt
	}
	fMgr.FMapRWMutex.Unlock()
	return nil
}
