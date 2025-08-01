// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nas

import (
	ctxt "context"
	"os"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/nas/nas_security"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	"github.com/omec-project/openapi/models"
)

func HandleNAS(ctx ctxt.Context, ue *context.RanUe, procedureCode int64, nasPdu []byte) {
	amfSelf := context.AMF_Self()

	if ue == nil {
		logger.NasLog.Errorln("RanUe is nil")
		return
	}

	if nasPdu == nil {
		ue.Log.Errorln("nasPdu is nil")
		return
	}

	if ue.AmfUe == nil {
		ue.AmfUe = nas_security.FetchUeContextWithMobileIdentity(nasPdu)
		if ue.AmfUe == nil {
			ue.AmfUe = amfSelf.NewAmfUe("")
		} else {
			if amfSelf.EnableSctpLb && amfSelf.EnableDbStore {
				/* checking the guti-ue belongs to this amf instance */
				id, err := amfSelf.Drsm.FindOwnerInt32ID(ue.AmfUe.Tmsi)
				if err != nil {
					logger.NasLog.Errorf("error checking guti-ue: %v", err)
				}
				if id != nil && id.PodName != os.Getenv("HOSTNAME") {
					rsp := &sdcoreAmfServer.AmfMessage{}
					rsp.VerboseMsg = "Redirecting Msg From AMF Pod !"
					rsp.Msgtype = sdcoreAmfServer.MsgType_REDIRECT_MSG
					rsp.AmfId = os.Getenv("HOSTNAME")
					/* TODO for this release setting pod ip to simplify logic in sctplb */
					rsp.RedirectId = id.PodIp
					rsp.GnbId = ue.Ran.GnbId
					rsp.Msg = ue.SctplbMsg
					if ue.AmfUe != nil {
						ue.AmfUe.Remove()
					} else {
						if err := ue.Remove(); err != nil {
							logger.NasLog.Errorf("error removing ue: %v", err)
						}
					}
					ue.Ran.Amf2RanMsgChan <- rsp
					return
				}
			}
		}

		ue.AmfUe.Mutex.Lock()
		defer ue.AmfUe.Mutex.Unlock()

		ue.Log.Infoln("Antype from new RanUe:", ue.Ran.AnType)
		// AnType is set in SetRanId function. This is called
		// when we handle NGSetup. In case of sctplb enabled,
		// we dont call this function when AMF restarts. So we
		// need to set the AnType from stored Information.
		if amfSelf.EnableSctpLb {
			ue.Ran.AnType = models.AccessType__3_GPP_ACCESS
		}
		ue.AmfUe.AttachRanUe(ue)

		if ue.AmfUe.EventChannel == nil {
			ue.AmfUe.EventChannel = ue.AmfUe.NewEventChannel()
			ue.AmfUe.EventChannel.UpdateNasHandler(DispatchMsg)
			go ue.AmfUe.EventChannel.Start(ctx)
		}
		ue.AmfUe.EventChannel.UpdateNasHandler(DispatchMsg)

		nasMsg := context.NasMsg{
			Context:       ctx,
			AnType:        ue.Ran.AnType,
			NasMsg:        nasPdu,
			ProcedureCode: procedureCode,
		}
		ue.AmfUe.EventChannel.SubmitMessage(nasMsg)

		return
	}
	if amfSelf.EnableSctpLb {
		ue.Ran.AnType = models.AccessType__3_GPP_ACCESS
	}

	msg, err := nas_security.Decode(ue.AmfUe, ue.Ran.AnType, nasPdu)
	if err != nil {
		ue.AmfUe.NASLog.Errorln(err)
		return
	}
	if err := Dispatch(ctx, ue.AmfUe, ue.Ran.AnType, procedureCode, msg); err != nil {
		ue.AmfUe.NASLog.Errorf("handle NAS Error: %v", err)
	}
}

func DispatchMsg(amfUe *context.AmfUe, transInfo context.NasMsg) {
	amfUe.NASLog.Infoln("handle Nas Message")
	msg, err := nas_security.Decode(amfUe, transInfo.AnType, transInfo.NasMsg)
	if err != nil {
		amfUe.NASLog.Errorln(err)
		return
	}

	if err := Dispatch(transInfo.Context, amfUe, transInfo.AnType, transInfo.ProcedureCode, msg); err != nil {
		amfUe.NASLog.Errorf("handle NAS Error: %v", err)
	}
}
