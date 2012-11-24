package topology

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"pkg/storage"
	"pkg/util"
	"time"
)

func batchVacuumVolumeCheck(vl *VolumeLayout, vid storage.VolumeId, locationlist *VolumeLocationList) bool {
	ch := make(chan bool, locationlist.Length())
	for index, dn := range locationlist.list {
		go func(index int, url string, vid storage.VolumeId) {
			//fmt.Println(index, "Check vacuuming", vid, "on", dn.Url())
			if e, ret := vacuumVolume_Check(url, vid); e != nil {
				//fmt.Println(index, "Error when checking vacuuming", vid, "on", url, e)
				ch <- false
			} else {
				//fmt.Println(index, "Checked vacuuming", vid, "on", url)
				ch <- ret
			}
		}(index, dn.Url(), vid)
	}
	isCheckSuccess := true
	for _ = range locationlist.list {
		select {
		case canVacuum := <-ch:
			isCheckSuccess = isCheckSuccess && canVacuum
		case <-time.After(30 * time.Minute):
			isCheckSuccess = false
			break
		}
	}
	return isCheckSuccess
}
func batchVacuumVolumeCompact(vl *VolumeLayout, vid storage.VolumeId, locationlist *VolumeLocationList) bool {
	vl.removeFromWritable(vid)
	ch := make(chan bool, locationlist.Length())
	for index, dn := range locationlist.list {
		go func(index int, url string, vid storage.VolumeId) {
			fmt.Println(index, "Start vacuuming", vid, "on", dn.Url())
			if e := vacuumVolume_Compact(url, vid); e != nil {
				fmt.Println(index, "Error when vacuuming", vid, "on", url, e)
				ch <- false
			} else {
				fmt.Println(index, "Complete vacuuming", vid, "on", url)
				ch <- true
			}
		}(index, dn.Url(), vid)
	}
	isVacuumSuccess := true
	for _ = range locationlist.list {
		select {
		case _ = <-ch:
		case <-time.After(30 * time.Minute):
			isVacuumSuccess = false
			break
		}
	}
	return isVacuumSuccess
}
func batchVacuumVolumeCommit(vl *VolumeLayout, vid storage.VolumeId, locationlist *VolumeLocationList) bool {
	isCommitSuccess := true
	for _, dn := range locationlist.list {
		fmt.Println("Start Commiting vacuum", vid, "on", dn.Url())
		if e := vacuumVolume_Commit(dn.Url(), vid); e != nil {
			fmt.Println("Error when committing vacuum", vid, "on", dn.Url(), e)
			isCommitSuccess = false
		} else {
			fmt.Println("Complete Commiting vacuum", vid, "on", dn.Url())
		}
	}
	if isCommitSuccess {
		vl.setVolumeWritable(vid)
	}
	return isCommitSuccess
}
func (t *Topology) Vacuum() int {
	for _, vl := range t.replicaType2VolumeLayout {
		if vl != nil {
			for vid, locationlist := range vl.vid2location {
				if batchVacuumVolumeCheck(vl, vid, locationlist) {
					if batchVacuumVolumeCompact(vl, vid, locationlist) {
						batchVacuumVolumeCommit(vl, vid, locationlist)
					}
				}
			}
		}
	}
	return 0
}

type VacuumVolumeResult struct {
	Result bool
	Error  string
}

func vacuumVolume_Check(urlLocation string, vid storage.VolumeId) (error, bool) {
	values := make(url.Values)
	values.Add("volume", vid.String())
    values.Add("garbageThreshold", "0.3")
	jsonBlob, err := util.Post("http://"+urlLocation+"/admin/vacuum_volume_check", values)
	if err != nil {
		return err, false
	}
	var ret VacuumVolumeResult
	if err := json.Unmarshal(jsonBlob, &ret); err != nil {
		return err, false
	}
	if ret.Error != "" {
		return errors.New(ret.Error), false
	}
	return nil, ret.Result
}
func vacuumVolume_Compact(urlLocation string, vid storage.VolumeId) error {
	values := make(url.Values)
	values.Add("volume", vid.String())
	jsonBlob, err := util.Post("http://"+urlLocation+"/admin/vacuum_volume_compact", values)
	if err != nil {
		return err
	}
	var ret VacuumVolumeResult
	if err := json.Unmarshal(jsonBlob, &ret); err != nil {
		return err
	}
	if ret.Error != "" {
		return errors.New(ret.Error)
	}
	return nil
}
func vacuumVolume_Commit(urlLocation string, vid storage.VolumeId) error {
	values := make(url.Values)
	values.Add("volume", vid.String())
	jsonBlob, err := util.Post("http://"+urlLocation+"/admin/vacuum_volume_commit", values)
	if err != nil {
		return err
	}
	var ret VacuumVolumeResult
	if err := json.Unmarshal(jsonBlob, &ret); err != nil {
		return err
	}
	if ret.Error != "" {
		return errors.New(ret.Error)
	}
	return nil
}