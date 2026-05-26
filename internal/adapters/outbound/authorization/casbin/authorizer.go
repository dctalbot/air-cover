package casbin

import (
	"context"
	"database/sql"
	"fmt"

	sqladapter "github.com/Blank-Xu/sql-adapter"
	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"

	appauthz "air-cover/internal/app/authorization"
	"air-cover/internal/apperrors"
)

const (
	sqliteDriverName = "sqlite3"
	tableName        = "casbin_rule"

	ruleAdmin                     = "admin"
	ruleAdminExceptSelfDeactivate = "admin_except_self_deactivate"
	ruleOwnerOrAdmin              = "owner_or_admin"
	ruleOpenNonOwner              = "open_non_owner"
	ruleTaker                     = "taker"

	matchesRuleFunction = "matchesRule"
)

var (
	newSQLAdapter      = sqladapter.NewAdapterWithContext
	newModelFromString = casbinmodel.NewModelFromString
	newEnforcer        = casbin.NewEnforcer
	addBaselinePolicy  = func(enforcer *casbin.Enforcer) error {
		_, err := enforcer.AddPoliciesEx(baselinePolicies)
		return err
	}
	seedPolicy = seedBaselinePolicy
)

type Authorizer struct {
	enforcer *casbin.Enforcer
}

func NewAuthorizer(ctx context.Context, db *sql.DB) (*Authorizer, error) {
	adapter, err := newSQLAdapter(ctx, db, sqliteDriverName, tableName)
	if err != nil {
		return nil, fmt.Errorf("create casbin sql adapter: %w", err)
	}
	model, err := newModelFromString(modelText)
	if err != nil {
		return nil, fmt.Errorf("create casbin model: %w", err)
	}
	enforcer, err := newEnforcer(model, adapter)
	if err != nil {
		return nil, fmt.Errorf("create casbin enforcer: %w", err)
	}
	registerMatcherFunctions(enforcer)
	if err := seedPolicy(ctx, enforcer); err != nil {
		return nil, err
	}
	return &Authorizer{enforcer: enforcer}, nil
}

func (a *Authorizer) Authorize(ctx context.Context, subject appauthz.Subject, action appauthz.Action, resource appauthz.Resource) error {
	allowed, err := a.enforcer.Enforce(enforcementSubjectFrom(subject), string(action), enforcementResourceFrom(resource))
	if err != nil {
		return fmt.Errorf("enforce authorization policy: %w", err)
	}
	if !allowed {
		return apperrors.ErrForbidden
	}
	return nil
}

func seedBaselinePolicy(ctx context.Context, enforcer *casbin.Enforcer) error {
	_ = ctx
	if err := enforcer.LoadPolicy(); err != nil {
		return fmt.Errorf("load casbin policy before seed: %w", err)
	}
	if err := addBaselinePolicy(enforcer); err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

var baselinePolicies = [][]string{
	{string(appauthz.ActionAdminAccess), string(appauthz.ResourceAdmin), ruleAdmin},
	{string(appauthz.ActionUserList), string(appauthz.ResourceAdmin), ruleAdmin},
	{string(appauthz.ActionUserCreate), string(appauthz.ResourceAdmin), ruleAdmin},
	{string(appauthz.ActionUserUpdate), string(appauthz.ResourceUser), ruleAdminExceptSelfDeactivate},
	{string(appauthz.ActionSubRequestDelete), string(appauthz.ResourceSubRequest), ruleOwnerOrAdmin},
	{string(appauthz.ActionSubRequestTake), string(appauthz.ResourceSubRequest), ruleOpenNonOwner},
	{string(appauthz.ActionSubRequestUntake), string(appauthz.ResourceSubRequest), ruleTaker},
}

const modelText = `
[request_definition]
r = sub, act, obj

[policy_definition]
p = action, resource_type, rule

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.act == p.action && r.obj.Type == p.resource_type && matchesRule(p.rule, r.sub, r.obj)
`

func registerMatcherFunctions(enforcer *casbin.Enforcer) {
	enforcer.AddFunction(matchesRuleFunction, casbinMatchesRule)
}

func casbinMatchesRule(args ...interface{}) (interface{}, error) {
	if len(args) != 3 {
		return false, fmt.Errorf("%s expects rule, subject, and resource", matchesRuleFunction)
	}
	rule, ok := args[0].(string)
	if !ok {
		return false, fmt.Errorf("%s rule argument must be string", matchesRuleFunction)
	}
	subject, ok := args[1].(enforcementSubject)
	if !ok {
		return false, fmt.Errorf("%s subject argument must be enforcementSubject", matchesRuleFunction)
	}
	resource, ok := args[2].(enforcementResource)
	if !ok {
		return false, fmt.Errorf("%s resource argument must be enforcementResource", matchesRuleFunction)
	}
	return matchesRule(rule, subject, resource), nil
}

func matchesRule(rule string, subject enforcementSubject, resource enforcementResource) bool {
	switch rule {
	case ruleAdmin:
		return isAdmin(subject)
	case ruleAdminExceptSelfDeactivate:
		return adminExceptSelfDeactivate(subject, resource)
	case ruleOwnerOrAdmin:
		return ownerOrAdmin(subject, resource)
	case ruleOpenNonOwner:
		return openNonOwner(subject, resource)
	case ruleTaker:
		return taker(subject, resource)
	default:
		return false
	}
}

func isAdmin(subject enforcementSubject) bool {
	return subject.IsAdmin
}

func adminExceptSelfDeactivate(subject enforcementSubject, resource enforcementResource) bool {
	return subject.IsAdmin && (!resource.DisableTarget || resource.TargetUserID != subject.UserID)
}

func ownerOrAdmin(subject enforcementSubject, resource enforcementResource) bool {
	return resource.OwnerUserID == subject.UserID || subject.IsAdmin
}

func openNonOwner(subject enforcementSubject, resource enforcementResource) bool {
	return resource.SubRequestOpen && resource.OwnerUserID != subject.UserID
}

func taker(subject enforcementSubject, resource enforcementResource) bool {
	return resource.HasTaker && resource.TakerUserID == subject.UserID
}

type enforcementSubject struct {
	UserID  int
	IsAdmin bool
}

func enforcementSubjectFrom(subject appauthz.Subject) enforcementSubject {
	return enforcementSubject{
		UserID:  subject.User.ID,
		IsAdmin: subject.User.IsAdmin(),
	}
}

type enforcementResource struct {
	Type           string
	OwnerUserID    int
	TakerUserID    int
	HasTaker       bool
	TargetUserID   int
	DisableTarget  bool
	SubRequestOpen bool
}

func enforcementResourceFrom(resource appauthz.Resource) enforcementResource {
	return enforcementResource{
		Type:           string(resource.Type),
		OwnerUserID:    resource.OwnerUserID,
		TakerUserID:    resource.TakerUserID,
		HasTaker:       resource.HasTaker,
		TargetUserID:   resource.TargetUserID,
		DisableTarget:  resource.DisableTarget,
		SubRequestOpen: resource.SubRequestOpen,
	}
}
