package api2go

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/artpar/api2go/v2/jsonapi"
)

type linkageParent struct {
	model jsonapi.MarshalIdentifier
	calls int
}

func (p *linkageParent) FindOne(string, Request) (Responder, error) {
	p.calls++
	return Response{Res: p.model, Code: http.StatusOK}, nil
}

type linkagePager struct {
	linkageParent
	pageCalls int
	fallback  bool
	shared    *jsonapi.Relationship
}

func (p *linkagePager) FindRelationship(_ string, _ jsonapi.Reference, req Request) (uint, Responder, error) {
	p.pageCalls++
	if p.fallback {
		return 0, nil, nil
	}
	rel := jsonapi.Relationship{Data: &jsonapi.RelationshipDataContainer{
		DataArray: []jsonapi.RelationshipData{{Type: "child", ID: "parent-page"}},
	}}
	if p.shared != nil {
		rel = *p.shared
	}
	pagination := Pagination{Next: map[string]string{"number": "2", "size": "1"}}
	if req.QueryParams["page[number]"] != nil && req.QueryParams["page[number]"][0] == "2" {
		pagination = Pagination{Prev: map[string]string{"number": "1", "size": "1"}}
	}
	return 2, Response{Res: rel, Code: http.StatusOK, Meta: map[string]interface{}{"response": "retained"}, Pagination: pagination}, nil
}

type linkageTarget struct {
	calls int
	err   error
}

type staticLinkagePager struct {
	response Responder
}

func (p staticLinkagePager) FindRelationship(string, jsonapi.Reference, Request) (uint, Responder, error) {
	return 3, p.response, nil
}

type cachedLinksResponse struct {
	Response
	links jsonapi.Links
}

func (r cachedLinksResponse) Links(*http.Request, string) jsonapi.Links {
	return r.links
}

func (s *linkageTarget) FindAll(Request) (Responder, error) {
	s.calls++
	return Response{Res: nil, Code: http.StatusOK}, s.err
}

type linkageModel struct {
	Api2GoModel
	refs []jsonapi.Reference
	ids  []jsonapi.ReferenceID
}

func (m linkageModel) GetReferences() []jsonapi.Reference      { return m.refs }
func (m linkageModel) GetReferencedIDs() []jsonapi.ReferenceID { return m.ids }
func (m linkageModel) GetCustomMeta(string) map[string]jsonapi.Meta {
	return map[string]jsonapi.Meta{"children": {"custom": "retained"}}
}

func linkageFixture(ids []jsonapi.ReferenceID, target interface{}) (*resource, *linkageParent, jsonapi.Reference) {
	ref := jsonapi.Reference{Type: "child", Name: "children", Relationship: jsonapi.ToManyRelationship}
	parent := &linkageParent{model: linkageModel{
		Api2GoModel: NewApi2GoModelWithData("parent", nil, 0, nil, map[string]interface{}{"reference_id": "parent-1"}),
		refs:        []jsonapi.Reference{ref}, ids: ids,
	}}
	api := NewAPI("")
	res := &resource{name: "parent", source: parent, api: api}
	if target != nil {
		api.resources = append(api.resources, resource{name: "child", source: target})
	}
	return res, parent, ref
}

func readLinkage(t *testing.T, res *resource, ref jsonapi.Reference, path string) map[string]interface{} {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if err := res.handleReadRelation(&APIContext{}, w, r, map[string]string{"id": "parent-1"}, res.api.info, ref); err != nil {
		t.Fatal(err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRelationshipLinkageKeepsParentAuthority(t *testing.T) {
	ids := []jsonapi.ReferenceID{{Type: "child", Name: "children", ID: "parent-link", Relationship: jsonapi.ToManyRelationship}}
	for _, withLink := range []bool{false, true} {
		var target interface{}
		if withLink {
			target = &linkageTarget{err: errors.New("target unavailable")}
		}
		res, parent, ref := linkageFixture(ids, target)
		body := readLinkage(t, res, ref, "/parent/parent-1/relationships/children")
		if got := body["data"].([]interface{})[0].(map[string]interface{})["id"]; got != "parent-link" {
			t.Fatalf("parent link replaced: %v", got)
		}
		if body["meta"].(map[string]interface{})["custom"] != "retained" {
			t.Fatalf("custom metadata lost: %#v", body)
		}
		if parent.calls != 1 {
			t.Fatalf("parent read count: %d", parent.calls)
		}
		if withLink && target.(*linkageTarget).calls != 0 {
			t.Fatal("target collection was read")
		}
		paged := readLinkage(t, res, ref, "/parent/parent-1/relationships/children?page[number]=1&page[size]=1")
		if got := paged["data"].([]interface{})[0].(map[string]interface{})["id"]; got != "parent-link" {
			t.Fatalf("page query replaced parent linkage: %v", got)
		}
		if withLink && target.(*linkageTarget).calls != 0 {
			t.Fatal("paged linkage read the target collection")
		}
		res, _, ref = linkageFixture(nil, target)
		body = readLinkage(t, res, ref, "/parent/parent-1/relationships/children")
		if got := body["data"].([]interface{}); len(got) != 0 {
			t.Fatalf("known empty link replaced: %#v", got)
		}
	}
}

func TestRelationshipParentPagerPreservesCustomLinks(t *testing.T) {
	res, ordinary, ref := linkageFixture(nil, &linkageTarget{err: errors.New("target unavailable")})
	pager := &linkagePager{linkageParent: *ordinary}
	res.source = pager
	body := readLinkage(t, res, ref, "/parent/parent-1/relationships/children?page[number]=1&page[size]=1")
	if got := body["data"].([]interface{})[0].(map[string]interface{})["id"]; got != "parent-page" {
		t.Fatalf("pager ID: %v", got)
	}
	links := body["links"].(map[string]interface{})
	if links["next"] == nil || links["last_page"] != nil || links["next_page_url"] != nil {
		t.Fatalf("pagination links lost: %#v", links)
	}
	if pager.pageCalls != 1 || pager.calls != 0 {
		t.Fatalf("parent calls page=%d ordinary=%d", pager.pageCalls, pager.calls)
	}
	body = readLinkage(t, res, ref, "/parent/parent-1/relationships/children?page[offset]=0&page[limit]=1")
	if got := body["data"].([]interface{})[0].(map[string]interface{})["id"]; got != "parent-page" || pager.pageCalls != 2 || pager.calls != 0 {
		t.Fatalf("offset request bypassed parent pager: %#v", body)
	}
}

func TestRelationshipParentPagerDoesNotMutateSharedResponse(t *testing.T) {
	res, ordinary, ref := linkageFixture(nil, nil)
	shared := jsonapi.Relationship{
		Data:  &jsonapi.RelationshipDataContainer{DataArray: []jsonapi.RelationshipData{{Type: "child", ID: "parent-page"}}},
		Meta:  map[string]interface{}{"source": "retained"},
		Links: jsonapi.Links{"self": jsonapi.Link{Href: "/parent/parent-1/relationships/children"}},
	}
	res.source = &linkagePager{linkageParent: *ordinary, shared: &shared}
	second := readLinkage(t, res, ref, "/parent/parent-1/relationships/children?page[number]=2&page[size]=1")
	first := readLinkage(t, res, ref, "/parent/parent-1/relationships/children?page[number]=1&page[size]=1")
	if second["links"].(map[string]interface{})["prev"] == nil || first["links"].(map[string]interface{})["prev"] != nil {
		t.Fatalf("pagination links leaked across responses: second=%#v first=%#v", second, first)
	}
	if len(shared.Links) != 1 || shared.Links["prev"] != nil || shared.Links["next"] != nil || len(shared.Meta) != 1 {
		t.Fatalf("source relationship was mutated: %#v", shared)
	}
}

func TestRelationshipSelfLinkKeepsGeneratedPaginationWithoutMutatingSource(t *testing.T) {
	res, _, ref := linkageFixture(nil, nil)
	shared := jsonapi.Relationship{
		Data:  &jsonapi.RelationshipDataContainer{DataArray: []jsonapi.RelationshipData{{Type: "child", ID: "child-1"}}},
		Links: jsonapi.Links{"self": jsonapi.Link{Href: "/parent/parent-1/relationships/children"}},
		Meta:  map[string]interface{}{"source": true},
	}
	res.source = staticLinkagePager{response: Response{Res: shared, Meta: map[string]interface{}{"response": true}}}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			writer := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/parent/parent-1/relationships/children?page[number]=1&page[size]=1", nil)
			if err := res.handleReadRelation(&APIContext{}, writer, request, map[string]string{"id": "parent-1"}, res.api.info, ref); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	body := readLinkage(t, res, ref, "/parent/parent-1/relationships/children?page[number]=1&page[size]=1")
	links := body["links"].(map[string]interface{})
	if links["self"] == nil || links["next_page_url"] == nil || len(shared.Links) != 1 || len(shared.Meta) != 1 {
		t.Fatalf("self link or generated pagination lost, or source mutated: %#v %#v", links, shared)
	}
}

func TestRelationshipResponderLinksAreReadOnly(t *testing.T) {
	res, _, ref := linkageFixture(nil, nil)
	shared := jsonapi.Links{"self": jsonapi.Link{Href: "/parent/parent-1/relationships/children"}}
	res.source = staticLinkagePager{response: cachedLinksResponse{
		Response: Response{Res: jsonapi.Relationship{Data: &jsonapi.RelationshipDataContainer{
			DataArray: []jsonapi.RelationshipData{{Type: "child", ID: "child-1"}},
		}}}, links: shared,
	}}
	second := readLinkage(t, res, ref, "/parent/parent-1/relationships/children?page[number]=2&page[size]=1")
	first := readLinkage(t, res, ref, "/parent/parent-1/relationships/children?page[number]=1&page[size]=1")
	if second["links"].(map[string]interface{})["current_page"] != float64(2) ||
		first["links"].(map[string]interface{})["current_page"] != float64(1) ||
		first["links"].(map[string]interface{})["prev_page_url"] != nil || len(shared) != 1 {
		t.Fatalf("responder links mutated or reused: first=%#v second=%#v source=%#v", first, second, shared)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(page int) {
			defer wg.Done()
			writer := httptest.NewRecorder()
			path := "/parent/parent-1/relationships/children?page[number]=1&page[size]=1"
			if page%2 == 0 {
				path = "/parent/parent-1/relationships/children?page[number]=2&page[size]=1"
			}
			request := httptest.NewRequest(http.MethodGet, path, nil)
			if err := res.handleReadRelation(&APIContext{}, writer, request, map[string]string{"id": "parent-1"}, res.api.info, ref); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if len(shared) != 1 {
		t.Fatalf("responder links changed after concurrent requests: %#v", shared)
	}
}

func TestRelationshipParentPagerRejectsToOnePayload(t *testing.T) {
	res, ordinary, ref := linkageFixture(nil, nil)
	res.source = &linkagePager{linkageParent: *ordinary, shared: &jsonapi.Relationship{
		Data: &jsonapi.RelationshipDataContainer{DataObject: &jsonapi.RelationshipData{Type: "child", ID: "child-1"}},
	}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/parent/parent-1/relationships/children", nil)
	if err := res.handleReadRelation(&APIContext{}, w, r, map[string]string{"id": "parent-1"}, res.api.info, ref); err == nil {
		t.Fatal("to-many reader accepted a single-object linkage")
	}
}

func TestRelationshipParentPagerCannotDelegateAtRuntime(t *testing.T) {
	ids := []jsonapi.ReferenceID{{Type: "child", Name: "children", ID: "parent-link", Relationship: jsonapi.ToManyRelationship}}
	res, ordinary, ref := linkageFixture(ids, nil)
	pager := &linkagePager{linkageParent: *ordinary, fallback: true}
	res.source = pager
	writer := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/parent/parent-1/relationships/children", nil)
	if err := res.handleReadRelation(&APIContext{}, writer, request, map[string]string{"id": "parent-1"}, res.api.info, ref); err == nil {
		t.Fatal("nil relationship response was accepted")
	}
	if pager.pageCalls != 1 || pager.calls != 0 {
		t.Fatalf("reader delegated to FindOne: page=%d ordinary=%d", pager.pageCalls, pager.calls)
	}
}
