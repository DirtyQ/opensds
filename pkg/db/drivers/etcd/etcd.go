// Copyright (c) 2017 Huawei Technologies Co., Ltd. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
This module implements the etcd database operation of data structure
defined in api module.

*/

package etcd

import (
	"encoding/json"
	"errors"
	"time"

	"fmt"
	"sort"
	"strconv"
	"strings"

	log "github.com/golang/glog"
	c "github.com/opensds/opensds/pkg/context"
	"github.com/opensds/opensds/pkg/model"
	"github.com/opensds/opensds/pkg/utils/constants"
	"github.com/opensds/opensds/pkg/utils/urls"
	"github.com/satori/go.uuid"
)

func IsAdminContext(ctx *c.Context) bool {
	return ctx.IsAdmin
}

func AuthorizeProjectContext(ctx *c.Context) bool {
	tenantId := strings.Split(ctx.Uri, "/")[1]
	return ctx.TenantId == tenantId
}

// NewClient
func NewClient(edps []string) *Client {
	return &Client{
		clientInterface: Init(edps),
	}
}

// Client
type Client struct {
	clientInterface
}

//Parameter
type Parameter struct {
	beginIdx, endIdx int
	sortDir, sortKey string
}

//IsInArray
func (c *Client) IsInArray(e string, s []string) bool {
	for _, v := range s {
		if strings.EqualFold(e, v) {
			return true
		}
	}
	return false
}

func (c *Client) SelectOrNot(m map[string][]string) bool {
	for key := range m {
		if key != "limit" && key != "offset" && key != "sortDir" && key != "sortKey" {
			return true
		}
	}
	return false
}

var LIMIT = 50

//Get parameter limit
func (c *Client) GetLimit(m map[string][]string) int {
	var limit int
	var err error
	v, ok := m["limit"]
	if ok {
		limit, err = strconv.Atoi(v[0])
		if err != nil || limit < 0 {
			log.Warning("Invalid input limit:", limit, ",use default value instead:50")
			return LIMIT
		}
	} else {
		log.Warning("The parameter limit is not present,use default value instead:50")
		return LIMIT
	}
	return limit
}

var OFFSET = 0

//Get parameter offset
func (c *Client) GetOffset(m map[string][]string, size int) int {

	var offset int
	var err error
	v, ok := m["offset"]
	if ok {
		offset, err = strconv.Atoi(v[0])

		if err != nil || offset < 0 || offset > size {
			log.Warning("Invalid input offset or input offset is out of bounds:", offset, ",use default value instead:0")

			return OFFSET
		}

	} else {
		log.Warning("The parameter offset is not present,use default value instead:0")
		return OFFSET
	}
	return offset
}

var SORTDIR = "desc"

//Get parameter sortDir
func (c *Client) GetSortDir(m map[string][]string) string {
	var sortDir string
	v, ok := m["sortDir"]
	if ok {
		sortDir = v[0]
		if !strings.EqualFold(sortDir, "desc") && !strings.EqualFold(sortDir, "asc") {
			log.Warning("Invalid input sortDir:", sortDir, ",use default value instead:desc")
			return SORTDIR
		}
	} else {
		log.Warning("The parameter sortDir is not present,use default value instead:desc")
		return SORTDIR
	}
	return sortDir
}

var SORTKEY = "ID"

//Get parameter sortKey
func (c *Client) GetSortKey(m map[string][]string, sort_keys []string) string {
	var sortKey string
	v, ok := m["sortKey"]
	if ok {
		sortKey = strings.ToUpper(v[0])
		if !c.IsInArray(sortKey, sort_keys) {
			log.Warning("Invalid input sortKey:", sortKey, ",use default value instead:ID")
			return SORTKEY
		}

	} else {
		log.Warning("The parameter sortKey is not present,use default value instead:ID")
		return SORTKEY
	}
	return sortKey
}

//ParameterFilter
func (c *Client) ParameterFilter(m map[string][]string, size int, sort_keys []string) *Parameter {

	limit := c.GetLimit(m)
	offset := c.GetOffset(m, size)

	beginIdx := offset
	endIdx := limit + offset

	if endIdx > size {
		endIdx = size
	}

	sortDir := c.GetSortDir(m)
	sortKey := c.GetSortKey(m, sort_keys)

	return &Parameter{beginIdx, endIdx, sortDir, sortKey}
}

// CreateDock
func (c *Client) CreateDock(ctx *c.Context, dck *model.DockSpec) (*model.DockSpec, error) {
	if dck.Id == "" {
		dck.Id = uuid.NewV4().String()
	}

	if dck.CreatedAt == "" {
		dck.CreatedAt = time.Now().Format(constants.TimeFormat)
	}

	dckBody, err := json.Marshal(dck)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:     urls.GenerateDockURL(urls.Etcd, "", dck.Id),
		Content: string(dckBody),
	}
	dbRes := c.Create(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When create dock in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	return dck, nil
}

// GetDock
func (c *Client) GetDock(ctx *c.Context, dckID string) (*model.DockSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateDockURL(urls.Etcd, "", dckID),
	}
	dbRes := c.Get(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When get dock in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var dck = &model.DockSpec{}
	if err := json.Unmarshal([]byte(dbRes.Message[0]), dck); err != nil {
		log.Error("When parsing dock in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return dck, nil
}

// GetDockByPoolId
func (c *Client) GetDockByPoolId(ctx *c.Context, poolId string) (*model.DockSpec, error) {
	pool, err := c.GetPool(ctx, poolId)
	if err != nil {
		log.Error("Get pool failed in db: ", err)
		return nil, err
	}

	docks, err := c.ListDocks(ctx)
	if err != nil {
		log.Error("List docks failed failed in db: ", err)
		return nil, err
	}
	for _, dock := range docks {
		if pool.DockId == dock.Id {
			return dock, nil
		}
	}
	return nil, errors.New("Get dock failed by pool id: " + poolId)
}

// ListDocks
func (c *Client) ListDocks(ctx *c.Context) ([]*model.DockSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateDockURL(urls.Etcd, ""),
	}
	dbRes := c.List(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When list docks in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var dcks = []*model.DockSpec{}
	if len(dbRes.Message) == 0 {
		return dcks, nil
	}
	for _, msg := range dbRes.Message {
		var dck = &model.DockSpec{}
		if err := json.Unmarshal([]byte(msg), dck); err != nil {
			log.Error("When parsing dock in db:", dbRes.Error)
			return nil, errors.New(dbRes.Error)
		}
		dcks = append(dcks, dck)
	}
	return dcks, nil
}

var dock_sortKey string

type DockSlice []*model.DockSpec

func (dock DockSlice) Len() int { return len(dock) }

func (dock DockSlice) Swap(i, j int) { dock[i], dock[j] = dock[j], dock[i] }

func (dock DockSlice) Less(i, j int) bool {
	switch dock_sortKey {

	case "ID":
		return dock[i].Id < dock[j].Id
	case "NAME":
		return dock[i].Name < dock[j].Name
	case "STATUS":
		return dock[i].Status < dock[j].Status
	case "ENDPOINT":
		return dock[i].Endpoint < dock[j].Endpoint
	case "DRIVERNAME":
		return dock[i].DriverName < dock[j].DriverName
	case "DESCRIPTION":
		return dock[i].Description < dock[j].Description
	}
	return false
}

func (c *Client) FindDockValue(k string, d *model.DockSpec) string {
	switch k {
	case "Id":
		return d.Id
	case "CreatedAt":
		return d.CreatedAt
	case "Name":
		return d.Name
	case "UpdatedAt":
		return d.UpdatedAt
	case "Description":
		return d.Description
	case "Status":
		return d.Status
	case "StorageType":
		return d.StorageType
	case "Endpoint":
		return d.Endpoint
	case "DriverName":
		return d.DriverName
	}
	return ""
}

func (c *Client) SelectDocks(m map[string][]string, docks []*model.DockSpec) []*model.DockSpec {
	if !c.SelectOrNot(m) {
		return docks
	}
	var dcks = []*model.DockSpec{}

	var flag bool
	for _, dock := range docks {
		flag = true
		for key := range m {
			v := c.FindDockValue(key, dock)
			if v != "" && !strings.EqualFold(m[key][0], v) {
				flag = false
				break
			}
		}
		if flag {
			dcks = append(dcks, dock)
		}
	}
	return dcks
}

func (c *Client) SortDocks(dcks []*model.DockSpec, p *Parameter) []*model.DockSpec {
	dock_sortKey = p.sortKey
	if strings.EqualFold(p.sortDir, "asc") {
		sort.Sort(DockSlice(dcks))
	} else {
		sort.Sort(sort.Reverse(DockSlice(dcks)))
	}
	return dcks
}

func (c *Client) ListDocksWithFilter(ctx *c.Context, m map[string][]string) ([]*model.DockSpec, error) {
	docks, err := c.ListDocks(ctx)
	if err != nil {
		log.Error("List docks failed: ", err.Error())
		return nil, err
	}
	dcks := c.SelectDocks(m, docks)

	p := c.ParameterFilter(m, len(dcks), []string{"ID", "NAME", "ENDPOINT", "DRIVERNAME", "DESCRIPTION", "STATUS"})
	return c.SortDocks(dcks, p)[p.beginIdx:p.endIdx], nil
}

// UpdateDock
func (c *Client) UpdateDock(ctx *c.Context, dckID, name, desp string) (*model.DockSpec, error) {
	dck, err := c.GetDock(ctx, dckID)
	if err != nil {
		return nil, err
	}
	if name != "" {
		dck.Name = name
	}
	if desp != "" {
		dck.Description = desp
	}
	dck.UpdatedAt = time.Now().Format(constants.TimeFormat)

	dckBody, err := json.Marshal(dck)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:        urls.GenerateDockURL(urls.Etcd, "", dckID),
		NewContent: string(dckBody),
	}
	dbRes := c.Update(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When update dock in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return dck, nil
}

// DeleteDock
func (c *Client) DeleteDock(ctx *c.Context, dckID string) error {
	dbReq := &Request{
		Url: urls.GenerateDockURL(urls.Etcd, "", dckID),
	}
	dbRes := c.Delete(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When delete dock in db:", dbRes.Error)
		return errors.New(dbRes.Error)
	}
	return nil
}

// CreatePool
func (c *Client) CreatePool(ctx *c.Context, pol *model.StoragePoolSpec) (*model.StoragePoolSpec, error) {
	if pol.Id == "" {
		pol.Id = uuid.NewV4().String()
	}

	if pol.CreatedAt == "" {
		pol.CreatedAt = time.Now().Format(constants.TimeFormat)
	}
	polBody, err := json.Marshal(pol)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:     urls.GeneratePoolURL(urls.Etcd, "", pol.Id),
		Content: string(polBody),
	}
	dbRes := c.Create(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When create pol in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	return pol, nil
}

var pool_sortKey string

type StoragePoolSlice []*model.StoragePoolSpec

func (pool StoragePoolSlice) Len() int { return len(pool) }

func (pool StoragePoolSlice) Swap(i, j int) { pool[i], pool[j] = pool[j], pool[i] }

func (pool StoragePoolSlice) Less(i, j int) bool {
	switch pool_sortKey {

	case "ID":
		return pool[i].Id < pool[j].Id
	case "NAME":
		return pool[i].Name < pool[j].Name
	case "STATUS":
		return pool[i].Status < pool[j].Status
	case "AVAILABILITYZONE":
		return pool[i].AvailabilityZone < pool[j].AvailabilityZone
	case "DOCKID":
		return pool[i].DockId < pool[j].DockId
	case "DESCRIPTION":
		return pool[i].Description < pool[j].Description
	}
	return false
}

func (c *Client) FindPoolValue(k string, p *model.StoragePoolSpec) string {
	switch k {
	case "Id":
		return p.Id
	case "CreatedAt":
		return p.CreatedAt
	case "UpdatedAt":
		return p.UpdatedAt
	case "Name":
		return p.Name
	case "Description":
		return p.Description
	case "Status":
		return p.Status
	case "DockId":
		return p.DockId
	case "AvailabilityZone":
		return p.AvailabilityZone
	case "TotalCapacity":
		return strconv.FormatInt(p.TotalCapacity, 10)
	case "FreeCapacity":
		return strconv.FormatInt(p.FreeCapacity, 10)
	case "StorageType":
		return p.StorageType
	}
	return ""
}

func (c *Client) SelectPools(m map[string][]string, pools []*model.StoragePoolSpec) []*model.StoragePoolSpec {

	if !c.SelectOrNot(m) {
		return pools
	}
	var pols = []*model.StoragePoolSpec{}
	var flag bool
	for _, pool := range pools {
		flag = true
		for key := range m {
			v := c.FindPoolValue(key, pool)
			if v != "" && !strings.EqualFold(m[key][0], v) {
				flag = false
				break
			}
		}
		if flag {
			pols = append(pols, pool)
		}
	}
	return pols
}

func (c *Client) SortPools(pools []*model.StoragePoolSpec, p *Parameter) []*model.StoragePoolSpec {

	pool_sortKey = p.sortKey

	if strings.EqualFold(p.sortDir, "asc") {
		sort.Sort(StoragePoolSlice(pools))
	} else {
		sort.Sort(sort.Reverse(StoragePoolSlice(pools)))
	}
	return pools
}

func (c *Client) ListPoolsWithFilter(ctx *c.Context, m map[string][]string) ([]*model.StoragePoolSpec, error) {
	pools, err := c.ListPools(ctx)
	if err != nil {
		log.Error("List pools failed: ", err.Error())
		return nil, err
	}
	pols := c.SelectPools(m, pools)
	p := c.ParameterFilter(m, len(pols), []string{"ID", "NAME", "STATUS", "AVAILABILITYZONE", "DOCKID", "DESCRIPTION"})
	return c.SortPools(pols, p)[p.beginIdx:p.endIdx], nil

}

// GetPool
func (c *Client) GetPool(ctx *c.Context, polID string) (*model.StoragePoolSpec, error) {
	dbReq := &Request{
		Url: urls.GeneratePoolURL(urls.Etcd, "", polID),
	}
	dbRes := c.Get(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When get pool in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var pol = &model.StoragePoolSpec{}
	if err := json.Unmarshal([]byte(dbRes.Message[0]), pol); err != nil {
		log.Error("When parsing pool in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return pol, nil
}

// ListPools
func (c *Client) ListPools(ctx *c.Context) ([]*model.StoragePoolSpec, error) {
	dbReq := &Request{
		Url: urls.GeneratePoolURL(urls.Etcd, ""),
	}
	dbRes := c.List(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When list pools in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var pols = []*model.StoragePoolSpec{}
	if len(dbRes.Message) == 0 {
		return pols, nil
	}
	for _, msg := range dbRes.Message {
		var pol = &model.StoragePoolSpec{}
		if err := json.Unmarshal([]byte(msg), pol); err != nil {
			log.Error("When parsing pool in db:", dbRes.Error)
			return nil, errors.New(dbRes.Error)
		}
		pols = append(pols, pol)
	}
	return pols, nil
}

// UpdatePool
func (c *Client) UpdatePool(ctx *c.Context, polID, name, desp string, usedCapacity int64, used bool) (*model.StoragePoolSpec, error) {
	pol, err := c.GetPool(ctx, polID)
	if err != nil {
		return nil, err
	}
	if name != "" {
		pol.Name = name
	}
	if desp != "" {
		pol.Description = desp
	}
	pol.UpdatedAt = time.Now().Format(constants.TimeFormat)

	polBody, err := json.Marshal(pol)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:        urls.GeneratePoolURL(urls.Etcd, "", polID),
		NewContent: string(polBody),
	}
	dbRes := c.Update(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When update pool in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return pol, nil
}

// DeletePool
func (c *Client) DeletePool(ctx *c.Context, polID string) error {
	dbReq := &Request{
		Url: urls.GeneratePoolURL(urls.Etcd, "", polID),
	}
	dbRes := c.Delete(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When delete pool in db:", dbRes.Error)
		return errors.New(dbRes.Error)
	}
	return nil
}

// CreateProfile
func (c *Client) CreateProfile(ctx *c.Context, prf *model.ProfileSpec) (*model.ProfileSpec, error) {

	if prf.Id == "" {
		prf.Id = uuid.NewV4().String()
	}

	if prf.CreatedAt == "" {
		prf.CreatedAt = time.Now().Format(constants.TimeFormat)
	}

	prfBody, err := json.Marshal(prf)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:     urls.GenerateProfileURL(urls.Etcd, "", prf.Id),
		Content: string(prfBody),
	}
	dbRes := c.Create(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When create profile in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	return prf, nil
}

// GetProfile
func (c *Client) GetProfile(ctx *c.Context, prfID string) (*model.ProfileSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateProfileURL(urls.Etcd, "", prfID),
	}
	dbRes := c.Get(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When get profile in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var prf = &model.ProfileSpec{}
	if err := json.Unmarshal([]byte(dbRes.Message[0]), prf); err != nil {
		log.Error("When parsing profile in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return prf, nil
}

// GetDefaultProfile
func (c *Client) GetDefaultProfile(ctx *c.Context) (*model.ProfileSpec, error) {
	profiles, err := c.ListProfiles(ctx)
	if err != nil {
		log.Error("Get default profile failed in db: ", err)
		return nil, err
	}

	for _, profile := range profiles {
		if profile.Name == "default" {
			return profile, nil
		}
	}
	return nil, errors.New("No default profile in db.")
}

// ListProfiles
func (c *Client) ListProfiles(ctx *c.Context) ([]*model.ProfileSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateProfileURL(urls.Etcd, ""),
	}

	dbRes := c.List(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When list profiles in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var prfs = []*model.ProfileSpec{}
	if len(dbRes.Message) == 0 {
		return prfs, nil
	}
	for _, msg := range dbRes.Message {
		var prf = &model.ProfileSpec{}
		if err := json.Unmarshal([]byte(msg), prf); err != nil {
			log.Error("When parsing profile in db:", dbRes.Error)
			return nil, errors.New(dbRes.Error)
		}
		prfs = append(prfs, prf)
	}
	return prfs, nil
}

var profile_sortKey string

type ProfileSlice []*model.ProfileSpec

func (profile ProfileSlice) Len() int { return len(profile) }

func (profile ProfileSlice) Swap(i, j int) { profile[i], profile[j] = profile[j], profile[i] }

func (profile ProfileSlice) Less(i, j int) bool {
	switch profile_sortKey {

	case "ID":
		return profile[i].Id < profile[j].Id
	case "NAME":
		return profile[i].Name < profile[j].Name
	case "DESCRIPTION":
		return profile[i].Description < profile[j].Description
	}
	return false
}

func (c *Client) FindProfileValue(k string, p *model.ProfileSpec) string {
	switch k {
	case "Id":
		return p.Id
	case "CreatedAt":
		return p.CreatedAt
	case "UpdatedAt":
		return p.UpdatedAt
	case "Name":
		return p.Name
	case "Description":
		return p.Description
	case "StorageType":
		return p.StorageType
	}
	return ""
}

func (c *Client) SortProfiles(profiles []*model.ProfileSpec, p *Parameter) []*model.ProfileSpec {

	profile_sortKey = p.sortKey

	if strings.EqualFold(p.sortDir, "asc") {
		sort.Sort(ProfileSlice(profiles))
	} else {
		sort.Sort(sort.Reverse(ProfileSlice(profiles)))
	}
	return profiles
}

func (c *Client) SelectProfiles(m map[string][]string, profiles []*model.ProfileSpec) []*model.ProfileSpec {

	if !c.SelectOrNot(m) {
		return profiles
	}
	var prfs = []*model.ProfileSpec{}
	var flag bool
	for _, profile := range profiles {
		flag = true
		for key := range m {
			v := c.FindProfileValue(key, profile)
			if v != "" && !strings.EqualFold(m[key][0], v) {
				flag = false
				break
			}
		}
		if flag {
			prfs = append(prfs, profile)
		}
	}
	return prfs

}

func (c *Client) ListProfilesWithFilter(ctx *c.Context, m map[string][]string) ([]*model.ProfileSpec, error) {
	profiles, err := c.ListProfiles(ctx)
	if err != nil {
		log.Error("List profiles failed: ", err)
		return nil, err
	}
	prfs := c.SelectProfiles(m, profiles)

	p := c.ParameterFilter(m, len(prfs), []string{"ID", "NAME", "DESCRIPTION"})

	return c.SortProfiles(prfs, p)[p.beginIdx:p.endIdx], nil
}

// UpdateProfile
func (c *Client) UpdateProfile(ctx *c.Context, prfID string, input *model.ProfileSpec) (*model.ProfileSpec, error) {
	prf, err := c.GetProfile(ctx, prfID)
	if err != nil {
		return nil, err
	}
	if name := input.Name; name != "" {
		prf.Name = name
	}
	if desp := input.Description; desp != "" {
		prf.Description = desp
	}
	prf.UpdatedAt = time.Now().Format(constants.TimeFormat)

	if props := input.Extras; len(props) != 0 {
		if prf.Extras == nil {
			prf.Extras = make(map[string]interface{})
		}
		for k, v := range props {
			prf.Extras[k] = v
		}
	}

	prf.UpdatedAt = time.Now().Format(constants.TimeFormat)

	prfBody, err := json.Marshal(prf)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:        urls.GenerateProfileURL(urls.Etcd, "", prfID),
		NewContent: string(prfBody),
	}
	dbRes := c.Update(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When update profile in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return prf, nil
}

// DeleteProfile
func (c *Client) DeleteProfile(ctx *c.Context, prfID string) error {
	dbReq := &Request{
		Url: urls.GenerateProfileURL(urls.Etcd, "", prfID),
	}
	dbRes := c.Delete(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When delete profile in db:", dbRes.Error)
		return errors.New(dbRes.Error)
	}
	return nil
}

// AddExtraProperty
func (c *Client) AddExtraProperty(ctx *c.Context, prfID string, ext model.ExtraSpec) (*model.ExtraSpec, error) {
	prf, err := c.GetProfile(ctx, prfID)
	if err != nil {
		return nil, err
	}

	if prf.Extras == nil {
		prf.Extras = make(map[string]interface{})
	}

	for k, v := range ext {
		prf.Extras[k] = v
	}

	prf.UpdatedAt = time.Now().Format(constants.TimeFormat)

	if _, err = c.CreateProfile(nil, prf); err != nil {
		return nil, err
	}
	return &prf.Extras, nil
}

// ListExtraProperties
func (c *Client) ListExtraProperties(ctx *c.Context, prfID string) (*model.ExtraSpec, error) {
	prf, err := c.GetProfile(ctx, prfID)
	if err != nil {
		return nil, err
	}
	return &prf.Extras, nil
}

// RemoveExtraProperty
func (c *Client) RemoveExtraProperty(ctx *c.Context, prfID, extraKey string) error {
	prf, err := c.GetProfile(ctx, prfID)
	if err != nil {
		return err
	}

	delete(prf.Extras, extraKey)
	if _, err = c.CreateProfile(nil, prf); err != nil {
		return err
	}
	return nil
}

// CreateVolume
func (c *Client) CreateVolume(ctx *c.Context, vol *model.VolumeSpec) (*model.VolumeSpec, error) {
	if vol.Id == "" {
		vol.Id = uuid.NewV4().String()
	}

	if vol.CreatedAt == "" {
		vol.CreatedAt = time.Now().Format(constants.TimeFormat)
	}
	volBody, err := json.Marshal(vol)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:     urls.GenerateVolumeURL(urls.Etcd, ctx.TenantId, vol.Id),
		Content: string(volBody),
	}
	dbRes := c.Create(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When create volume in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	return vol, nil
}

// GetVolume
func (c *Client) GetVolume(ctx *c.Context, volID string) (*model.VolumeSpec, error) {
	vol, err := c.getVolume(ctx, volID)
	if !IsAdminContext(ctx) || err == nil {
		return vol, err
	}
	vols, err := c.ListVolumes(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range vols {
		if v.Id == volID {
			return v, nil
		}
	}
	return nil, fmt.Errorf("specified volume(%s) can't find", volID)
}

func (c *Client) getVolume(ctx *c.Context, volID string) (*model.VolumeSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateVolumeURL(urls.Etcd, ctx.TenantId, volID),
	}
	dbRes := c.Get(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When get volume in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var vol = &model.VolumeSpec{}
	if err := json.Unmarshal([]byte(dbRes.Message[0]), vol); err != nil {
		log.Error("When parsing volume in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return vol, nil
}

// ListVolumes
func (c *Client) ListVolumes(ctx *c.Context) ([]*model.VolumeSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateVolumeURL(urls.Etcd, ctx.TenantId),
	}

	// list all volumes not just belong specified project.
	if IsAdminContext(ctx) {
		dbReq.Url = urls.GenerateVolumeURL(urls.Etcd, "")
	}

	dbRes := c.List(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When list volumes in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var vols = []*model.VolumeSpec{}
	if len(dbRes.Message) == 0 {
		return vols, nil
	}
	for _, msg := range dbRes.Message {
		var vol = &model.VolumeSpec{}
		if err := json.Unmarshal([]byte(msg), vol); err != nil {
			log.Error("When parsing volume in db:", dbRes.Error)
			return nil, errors.New(dbRes.Error)
		}
		vols = append(vols, vol)
	}
	return vols, nil
}

var volume_sortKey string

type VolumeSlice []*model.VolumeSpec

func (volume VolumeSlice) Len() int { return len(volume) }

func (volume VolumeSlice) Swap(i, j int) { volume[i], volume[j] = volume[j], volume[i] }

func (volume VolumeSlice) Less(i, j int) bool {
	switch volume_sortKey {

	case "ID":
		return volume[i].Id < volume[j].Id
	case "NAME":
		return volume[i].Name < volume[j].Name
	case "STATUS":
		return volume[i].Status < volume[j].Status
	case "AVAILABILITYZONE":
		return volume[i].AvailabilityZone < volume[j].AvailabilityZone
	case "PROFILEID":
		return volume[i].ProfileId < volume[j].ProfileId
	case "TENANTID":
		return volume[i].TenantId < volume[j].TenantId
	case "SIZE":
		return volume[i].Size < volume[j].Size
	case "POOLID":
		return volume[i].PoolId < volume[j].PoolId
	case "DESCRIPTION":
		return volume[i].Description < volume[j].Description
		// TODO:case "lun_id" (admin_only)
		// TODO:case "GroupId"
	}
	return false
}

func (c *Client) FindVolumeValue(k string, p *model.VolumeSpec) string {
	switch k {
	case "Id":
		return p.Id
	case "CreatedAt":
		return p.CreatedAt
	case "UpdatedAt":
		return p.UpdatedAt
	case "TenantId":
		return p.TenantId
	case "UserId":
		return p.UserId
	case "Name":
		return p.Name
	case "Description":
		return p.Description
	case "AvailabilityZone":
		return p.AvailabilityZone
	case "Size":
		return strconv.FormatInt(p.Size, 10)
	case "Status":
		return p.Status
	case "PoolId":
		return p.PoolId
	case "ProfileId":
		return p.ProfileId
	}
	return ""
}

func (c *Client) SelectVolumes(m map[string][]string, volumes []*model.VolumeSpec) []*model.VolumeSpec {

	if !c.SelectOrNot(m) {
		return volumes
	}
	var vols = []*model.VolumeSpec{}
	var flag bool
	for _, vol := range volumes {
		flag = true
		for key := range m {
			v := c.FindVolumeValue(key, vol)
			if v != "" && !strings.EqualFold(m[key][0], v) {
				flag = false
				break
			}
		}
		if flag {
			vols = append(vols, vol)
		}
	}
	return vols

}

func (c *Client) SortVolumes(volumes []*model.VolumeSpec, p *Parameter) []*model.VolumeSpec {

	volume_sortKey = p.sortKey

	if strings.EqualFold(p.sortDir, "asc") {
		sort.Sort(VolumeSlice(volumes))

	} else {
		sort.Sort(sort.Reverse(VolumeSlice(volumes)))
	}
	return volumes
}

func (c *Client) ListVolumesWithFilter(ctx *c.Context, m map[string][]string) ([]*model.VolumeSpec, error) {
	volumes, err := c.ListVolumes(ctx)
	if err != nil {
		log.Error("List volumes failed: ", err)
		return nil, err
	}
	vols := c.SelectVolumes(m, volumes)

	p := c.ParameterFilter(m, len(vols), []string{"ID", "NAME", "STATUS", "AVAILABILITYZONE", "PROFILEID", "PROJECTID", "SIZE", "POOLID", "DESCRIPTION"})

	return c.SortVolumes(vols, p)[p.beginIdx:p.endIdx], nil
}

// UpdateVolume ...
func (c *Client) UpdateVolume(ctx *c.Context, vol *model.VolumeSpec) (*model.VolumeSpec, error) {
	result, err := c.GetVolume(ctx, vol.Id)
	if err != nil {
		return nil, err
	}

	if vol.Name != "" {
		result.Name = vol.Name
	}

	if vol.Description != "" {
		result.Description = vol.Description
	}

	// Set update time
	result.UpdatedAt = time.Now().Format(constants.TimeFormat)

	body, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:        urls.GenerateVolumeURL(urls.Etcd, ctx.TenantId, vol.Id),
		NewContent: string(body),
	}

	dbRes := c.Update(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When update volume in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return result, nil
}

// DeleteVolume
func (c *Client) DeleteVolume(ctx *c.Context, volID string) error {
	dbReq := &Request{
		Url: urls.GenerateVolumeURL(urls.Etcd, ctx.TenantId, volID),
	}
	dbRes := c.Delete(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When delete volume in db:", dbRes.Error)
		return errors.New(dbRes.Error)
	}
	return nil
}

// ExtendVolume ...
func (c *Client) ExtendVolume(ctx *c.Context, vol *model.VolumeSpec) (*model.VolumeSpec, error) {
	result, err := c.GetVolume(ctx, vol.Id)
	if err != nil {
		return nil, err
	}

	if vol.Size > 0 {
		result.Size = vol.Size
	}

	// Set update time
	result.UpdatedAt = time.Now().Format(constants.TimeFormat)

	body, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:        urls.GenerateVolumeURL(urls.Etcd, ctx.TenantId, vol.Id),
		NewContent: string(body),
	}

	dbRes := c.Update(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When extend volume in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return result, nil
}

// CreateVolumeAttachment
func (c *Client) CreateVolumeAttachment(ctx *c.Context, attachment *model.VolumeAttachmentSpec) (*model.VolumeAttachmentSpec, error) {
	if attachment.Id == "" {
		attachment.Id = uuid.NewV4().String()
	}

	if attachment.CreatedAt == "" {
		attachment.CreatedAt = time.Now().Format(constants.TimeFormat)
	}

	atcBody, err := json.Marshal(attachment)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:     urls.GenerateAttachmentURL(urls.Etcd, ctx.TenantId, attachment.Id),
		Content: string(atcBody),
	}
	dbRes := c.Create(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When create volume attachment in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	return attachment, nil
}
func (c *Client) GetVolumeAttachment(ctx *c.Context, attachmentId string) (*model.VolumeAttachmentSpec, error) {
	attach, err := c.getVolumeAttachment(ctx, attachmentId)
	if !IsAdminContext(ctx) || err == nil {
		return attach, err
	}
	attachs, err := c.ListVolumeAttachments(ctx, attachmentId)
	if err != nil {
		return nil, err
	}
	for _, v := range attachs {
		if v.Id == attachmentId {
			return v, nil
		}
	}
	return nil, fmt.Errorf("specified volume attachment(%s) can't find", attachmentId)
}

// GetVolumeAttachment
func (c *Client) getVolumeAttachment(ctx *c.Context, attachmentId string) (*model.VolumeAttachmentSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateAttachmentURL(urls.Etcd, ctx.TenantId, attachmentId),
	}
	dbRes := c.Get(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When get volume attachment in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var atc = &model.VolumeAttachmentSpec{}
	if err := json.Unmarshal([]byte(dbRes.Message[0]), atc); err != nil {
		log.Error("When parsing volume attachment in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return atc, nil
}

// ListVolumeAttachments
func (c *Client) ListVolumeAttachments(ctx *c.Context, volumeId string) ([]*model.VolumeAttachmentSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateAttachmentURL(urls.Etcd, ctx.TenantId),
	}
	if IsAdminContext(ctx) {
		dbReq.Url = urls.GenerateAttachmentURL(urls.Etcd, "")
	}
	dbRes := c.List(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When list volume attachments in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var atcs = []*model.VolumeAttachmentSpec{}
	for _, msg := range dbRes.Message {
		var atc = &model.VolumeAttachmentSpec{}
		if err := json.Unmarshal([]byte(msg), atc); err != nil {
			log.Error("When parsing volume attachment in db:", dbRes.Error)
			return nil, errors.New(dbRes.Error)
		}

		if len(volumeId) == 0 || atc.VolumeId == volumeId {
			atcs = append(atcs, atc)
		}
	}
	return atcs, nil

}

var volumeAttachment_sortKey string

type VolumeAttachmentSlice []*model.VolumeAttachmentSpec

func (volumeAttachment VolumeAttachmentSlice) Len() int { return len(volumeAttachment) }

func (volumeAttachment VolumeAttachmentSlice) Swap(i, j int) {

	volumeAttachment[i], volumeAttachment[j] = volumeAttachment[j], volumeAttachment[i]
}

func (volumeAttachment VolumeAttachmentSlice) Less(i, j int) bool {
	switch volumeAttachment_sortKey {

	case "ID":
		return volumeAttachment[i].Id < volumeAttachment[j].Id
	case "VOLUMEID":
		return volumeAttachment[i].VolumeId < volumeAttachment[j].VolumeId
	case "STATUS":
		return volumeAttachment[i].Status < volumeAttachment[j].Status
	case "USERID":
		return volumeAttachment[i].UserId < volumeAttachment[j].UserId
	case "TENANTID":
		return volumeAttachment[i].TenantId < volumeAttachment[j].TenantId
	}
	return false
}

func (c *Client) FindAttachmentValue(k string, p *model.VolumeAttachmentSpec) string {
	switch k {
	case "Id":
		return p.Id
	case "CreatedAt":
		return p.CreatedAt
	case "UpdatedAte":
		return p.UpdatedAt
	case "TenantId":
		return p.TenantId
	case "UserId":
		return p.UserId
	case "VolumeId":
		return p.VolumeId
	case "Mountpoint":
		return p.Mountpoint
	case "Status":
		return p.Status
	}
	return ""
}

func (c *Client) SelectVolumeAttachments(m map[string][]string, attachments []*model.VolumeAttachmentSpec) []*model.VolumeAttachmentSpec {

	if !c.SelectOrNot(m) {
		return attachments
	}
	var atcs = []*model.VolumeAttachmentSpec{}
	var flag bool
	for _, attachment := range attachments {
		flag = true
		for key := range m {
			v := c.FindAttachmentValue(key, attachment)
			if v != "" && !strings.EqualFold(m[key][0], v) {
				flag = false
				break
			}
		}
		if flag {
			atcs = append(atcs, attachment)
		}
	}
	return atcs

}

func (c *Client) SortVolumeAttachments(attachments []*model.VolumeAttachmentSpec, p *Parameter) []*model.VolumeAttachmentSpec {

	volumeAttachment_sortKey = p.sortKey

	if strings.EqualFold(p.sortDir, "asc") {
		sort.Sort(VolumeAttachmentSlice(attachments))
	} else {
		sort.Sort(sort.Reverse(VolumeAttachmentSlice(attachments)))
	}
	return attachments

}

func (c *Client) ListVolumeAttachmentsWithFilter(ctx *c.Context, m map[string][]string) ([]*model.VolumeAttachmentSpec, error) {
	volumeId := m["VolumeId"][0]
	volumeAttachments, err := c.ListVolumeAttachments(ctx, volumeId)
	if err != nil {
		log.Error("List volumes failed: ", err)
		return nil, err
	}
	atcs := c.SelectVolumeAttachments(m, volumeAttachments)
	p := c.ParameterFilter(m, len(atcs), []string{"ID", "VOLUMEID", "STATUS", "USERID", "PROJECTID"})

	return c.SortVolumeAttachments(atcs, p)[p.beginIdx:p.endIdx], nil
}

// UpdateVolumeAttachment
func (c *Client) UpdateVolumeAttachment(ctx *c.Context, attachmentId string, attachment *model.VolumeAttachmentSpec) (*model.VolumeAttachmentSpec, error) {
	result, err := c.GetVolumeAttachment(ctx, attachmentId)
	if err != nil {
		return nil, err
	}
	if len(attachment.Mountpoint) > 0 {
		result.Mountpoint = attachment.Mountpoint
	}
	if len(attachment.Status) > 0 {
		result.Status = attachment.Status
	}
	if len(attachment.Platform) > 0 {
		result.Platform = attachment.Platform
	}
	if len(attachment.OsType) > 0 {
		result.OsType = attachment.OsType
	}
	if len(attachment.Ip) > 0 {
		result.Ip = attachment.Ip
	}
	if len(attachment.Host) > 0 {
		result.Host = attachment.Host
	}
	if len(attachment.Initiator) > 0 {
		result.Initiator = attachment.Initiator
	}
	if len(attachment.DriverVolumeType) > 0 {
		result.DriverVolumeType = attachment.DriverVolumeType
	}
	// Update metadata
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	for k, v := range attachment.Metadata {
		result.Metadata[k] = v
	}
	// Update onnectionData
	for k, v := range attachment.ConnectionData {
		result.ConnectionData[k] = v
	}
	// Set update time
	result.UpdatedAt = time.Now().Format(constants.TimeFormat)

	atcBody, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:        urls.GenerateAttachmentURL(urls.Etcd, ctx.TenantId, attachmentId),
		NewContent: string(atcBody),
	}
	dbRes := c.Update(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When update volume attachment in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return result, nil
}

// DeleteVolumeAttachment
func (c *Client) DeleteVolumeAttachment(ctx *c.Context, attachmentId string) error {
	dbReq := &Request{
		Url: urls.GenerateAttachmentURL(urls.Etcd, ctx.TenantId, attachmentId),
	}
	dbRes := c.Delete(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When delete volume attachment in db:", dbRes.Error)
		return errors.New(dbRes.Error)
	}
	return nil
}

// CreateVolumeSnapshot
func (c *Client) CreateVolumeSnapshot(ctx *c.Context, snp *model.VolumeSnapshotSpec) (*model.VolumeSnapshotSpec, error) {
	if snp.Id == "" {
		snp.Id = uuid.NewV4().String()
	}

	if snp.CreatedAt == "" {
		snp.CreatedAt = time.Now().Format(constants.TimeFormat)
	}
	snpBody, err := json.Marshal(snp)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:     urls.GenerateSnapshotURL(urls.Etcd, ctx.TenantId, snp.Id),
		Content: string(snpBody),
	}
	dbRes := c.Create(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When create volume snapshot in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	return snp, nil
}
func (c *Client) GetVolumeSnapshot(ctx *c.Context, snpID string) (*model.VolumeSnapshotSpec, error) {
	snap, err := c.getVolumeSnapshot(ctx, snpID)
	if !IsAdminContext(ctx) || err == nil {
		return snap, err
	}
	snaps, err := c.ListVolumeSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range snaps {
		if v.Id == snpID {
			return v, nil
		}
	}
	return nil, fmt.Errorf("specified volume snapshot(%s) can't find", snpID)
}

// GetVolumeSnapshot
func (c *Client) getVolumeSnapshot(ctx *c.Context, snpID string) (*model.VolumeSnapshotSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateSnapshotURL(urls.Etcd, ctx.TenantId, snpID),
	}
	dbRes := c.Get(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When get volume attachment in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var vs = &model.VolumeSnapshotSpec{}
	if err := json.Unmarshal([]byte(dbRes.Message[0]), vs); err != nil {
		log.Error("When parsing volume snapshot in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return vs, nil
}

// ListVolumeSnapshots
func (c *Client) ListVolumeSnapshots(ctx *c.Context) ([]*model.VolumeSnapshotSpec, error) {
	dbReq := &Request{
		Url: urls.GenerateSnapshotURL(urls.Etcd, ctx.TenantId),
	}
	if IsAdminContext(ctx) {
		dbReq.Url = urls.GenerateSnapshotURL(urls.Etcd, "")
	}
	dbRes := c.List(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When list volume snapshots in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}

	var vss = []*model.VolumeSnapshotSpec{}
	if len(dbRes.Message) == 0 {
		return vss, nil
	}
	for _, msg := range dbRes.Message {
		var vs = &model.VolumeSnapshotSpec{}
		if err := json.Unmarshal([]byte(msg), vs); err != nil {
			log.Error("When parsing volume snapshot in db:", dbRes.Error)
			return nil, errors.New(dbRes.Error)
		}
		vss = append(vss, vs)
	}
	return vss, nil
}

var volumeSnapshot_sortKey string

type VolumeSnapshotSlice []*model.VolumeSnapshotSpec

func (volumeSnapshot VolumeSnapshotSlice) Len() int { return len(volumeSnapshot) }

func (volumeSnapshot VolumeSnapshotSlice) Swap(i, j int) {

	volumeSnapshot[i], volumeSnapshot[j] = volumeSnapshot[j], volumeSnapshot[i]
}

func (volumeSnapshot VolumeSnapshotSlice) Less(i, j int) bool {
	switch volumeSnapshot_sortKey {

	case "ID":
		return volumeSnapshot[i].Id < volumeSnapshot[j].Id
	case "VOLUMEID":
		return volumeSnapshot[i].VolumeId < volumeSnapshot[j].VolumeId
	case "STATUS":
		return volumeSnapshot[i].Status < volumeSnapshot[j].Status
	case "USERID":
		return volumeSnapshot[i].UserId < volumeSnapshot[j].UserId
	case "TENANTID":
		return volumeSnapshot[i].TenantId < volumeSnapshot[j].TenantId
	case "SIZE":
		return volumeSnapshot[i].Size < volumeSnapshot[j].Size
		//TODO:case "GroupSnapshotId"
	}
	return false
}

func (c *Client) FindSnapshotsValue(k string, p *model.VolumeSnapshotSpec) string {
	switch k {
	case "Id":
		return p.Id
	case "CreatedAt":
		return p.CreatedAt
	case "UpdatedAte":
		return p.UpdatedAt
	case "TenantId":
		return p.TenantId
	case "UserId":
		return p.UserId
	case "Name":
		return p.Name
	case "Description":
		return p.Description
	case "Status":
		return p.Status
	case "Size":
		return strconv.FormatInt(p.Size, 10)
	case "VolumeId":
		return p.VolumeId
	}
	return ""
}

func (c *Client) SelectSnapshots(m map[string][]string, snapshots []*model.VolumeSnapshotSpec) []*model.VolumeSnapshotSpec {

	if !c.SelectOrNot(m) {
		return snapshots
	}
	var snps = []*model.VolumeSnapshotSpec{}
	var flag bool
	for _, snapshot := range snapshots {
		flag = true
		for key := range m {
			v := c.FindSnapshotsValue(key, snapshot)
			if v != "" && !strings.EqualFold(m[key][0], v) {
				flag = false
				break
			}
		}
		if flag {
			snps = append(snps, snapshot)
		}
	}
	return snps

}

func (c *Client) SortSnapshots(snapshots []*model.VolumeSnapshotSpec, p *Parameter) []*model.VolumeSnapshotSpec {

	volumeSnapshot_sortKey = p.sortKey

	if strings.EqualFold(p.sortDir, "asc") {
		sort.Sort(VolumeSnapshotSlice(snapshots))
	} else {
		sort.Sort(sort.Reverse(VolumeSnapshotSlice(snapshots)))
	}
	return snapshots

}

func (c *Client) ListVolumeSnapshotsWithFilter(ctx *c.Context, m map[string][]string) ([]*model.VolumeSnapshotSpec, error) {
	volumeSnapshots, err := c.ListVolumeSnapshots(ctx)
	if err != nil {
		log.Error("List volumeSnapshots failed: ", err)
		return nil, err
	}
	snps := c.SelectSnapshots(m, volumeSnapshots)
	p := c.ParameterFilter(m, len(snps), []string{"ID", "VOLUMEID", "STATUS", "USERID", "PROJECTID"})

	return c.SortSnapshots(snps, p)[p.beginIdx:p.endIdx], nil
}

// UpdateVolumeSnapshot
func (c *Client) UpdateVolumeSnapshot(ctx *c.Context, snpID string, snp *model.VolumeSnapshotSpec) (*model.VolumeSnapshotSpec, error) {
	result, err := c.GetVolumeSnapshot(ctx, snpID)
	if err != nil {
		return nil, err
	}

	if snp.Name != "" {
		result.Name = snp.Name
	}

	if snp.Description != "" {
		result.Description = snp.Description
	}

	// Set update time
	result.UpdatedAt = time.Now().Format(constants.TimeFormat)

	atcBody, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	dbReq := &Request{
		Url:        urls.GenerateSnapshotURL(urls.Etcd, ctx.TenantId, snpID),
		NewContent: string(atcBody),
	}

	dbRes := c.Update(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When update volume snapshot in db:", dbRes.Error)
		return nil, errors.New(dbRes.Error)
	}
	return result, nil
}

// DeleteVolumeSnapshot
func (c *Client) DeleteVolumeSnapshot(ctx *c.Context, snpID string) error {
	dbReq := &Request{
		Url: urls.GenerateSnapshotURL(urls.Etcd, ctx.TenantId, snpID),
	}
	dbRes := c.Delete(dbReq)
	if dbRes.Status != "Success" {
		log.Error("When delete volume snapshot in db:", dbRes.Error)
		return errors.New(dbRes.Error)
	}
	return nil
}
