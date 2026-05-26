package casbin

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	appauthz "air-cover/internal/app/authorization"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"

	sqladapter "github.com/Blank-Xu/sql-adapter"
	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	_ "github.com/mattn/go-sqlite3"
)

func TestAuthorizerPolicyDecisions(t *testing.T) {
	authorizer, db := newTestAuthorizer(t)
	defer db.Close()

	admin := appauthz.SubjectFromCurrentUser(domain.CurrentUser{ID: 1, Role: domain.RoleAdmin})
	member := appauthz.SubjectFromCurrentUser(domain.CurrentUser{ID: 2, Role: domain.RoleMember})

	takerID := 3
	tests := []struct {
		name      string
		subject   appauthz.Subject
		action    appauthz.Action
		resource  appauthz.Resource
		wantError error
	}{
		{
			name:     "admin access",
			subject:  admin,
			action:   appauthz.ActionAdminAccess,
			resource: appauthz.AdminResource(),
		},
		{
			name:      "member denied admin access",
			subject:   member,
			action:    appauthz.ActionAdminAccess,
			resource:  appauthz.AdminResource(),
			wantError: apperrors.ErrForbidden,
		},
		{
			name:     "admin updates other user",
			subject:  admin,
			action:   appauthz.ActionUserUpdate,
			resource: appauthz.UserResource(2, true),
		},
		{
			name:      "admin cannot deactivate self",
			subject:   admin,
			action:    appauthz.ActionUserUpdate,
			resource:  appauthz.UserResource(1, true),
			wantError: apperrors.ErrForbidden,
		},
		{
			name:     "owner deletes sub request",
			subject:  member,
			action:   appauthz.ActionSubRequestDelete,
			resource: appauthz.SubRequestResource(&domain.SubRequest{PostedByUserID: 2}),
		},
		{
			name:     "member takes open request from someone else",
			subject:  member,
			action:   appauthz.ActionSubRequestTake,
			resource: appauthz.SubRequestResource(&domain.SubRequest{PostedByUserID: 1}),
		},
		{
			name:      "member cannot take own request",
			subject:   member,
			action:    appauthz.ActionSubRequestTake,
			resource:  appauthz.SubRequestResource(&domain.SubRequest{PostedByUserID: 2}),
			wantError: apperrors.ErrForbidden,
		},
		{
			name:      "member cannot take already taken request",
			subject:   member,
			action:    appauthz.ActionSubRequestTake,
			resource:  appauthz.SubRequestResource(&domain.SubRequest{PostedByUserID: 1, TakenByUserID: &takerID}),
			wantError: apperrors.ErrForbidden,
		},
		{
			name:     "admin can take own open request",
			subject:  admin,
			action:   appauthz.ActionSubRequestTake,
			resource: appauthz.SubRequestResource(&domain.SubRequest{PostedByUserID: 1}),
		},
		{
			name:     "taker can untake",
			subject:  appauthz.SubjectFromCurrentUser(domain.CurrentUser{ID: takerID, Role: domain.RoleMember}),
			action:   appauthz.ActionSubRequestUntake,
			resource: appauthz.SubRequestResource(&domain.SubRequest{PostedByUserID: 1, TakenByUserID: &takerID}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authorizer.Authorize(context.Background(), tt.subject, tt.action, tt.resource)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Authorize error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

func TestAuthorizerDeniesUnknownPolicyRule(t *testing.T) {
	authorizer, db := newTestAuthorizer(t)
	defer db.Close()

	action := appauthz.Action("test:unknown-rule")
	added, err := authorizer.enforcer.AddPolicy(string(action), string(appauthz.ResourceAdmin), "unknown_rule")
	if err != nil {
		t.Fatalf("add unknown policy rule: %v", err)
	}
	if !added {
		t.Fatal("expected unknown policy rule to be added")
	}

	admin := appauthz.SubjectFromCurrentUser(domain.CurrentUser{ID: 1, Role: domain.RoleAdmin})
	err = authorizer.Authorize(context.Background(), admin, action, appauthz.AdminResource())
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("Authorize error = %v, want %v", err, apperrors.ErrForbidden)
	}
}

func TestMatchesRule(t *testing.T) {
	admin := enforcementSubjectFrom(appauthz.SubjectFromCurrentUser(domain.CurrentUser{ID: 1, Role: domain.RoleAdmin}))
	member := enforcementSubjectFrom(appauthz.SubjectFromCurrentUser(domain.CurrentUser{ID: 2, Role: domain.RoleMember}))

	tests := []struct {
		name     string
		rule     string
		subject  enforcementSubject
		resource enforcementResource
		want     bool
	}{
		{name: "admin", rule: ruleAdmin, subject: admin, want: true},
		{name: "admin denied", rule: ruleAdmin, subject: member},
		{
			name:    "admin except self deactivate",
			rule:    ruleAdminExceptSelfDeactivate,
			subject: admin,
			resource: enforcementResource{
				TargetUserID:  2,
				DisableTarget: true,
			},
			want: true,
		},
		{
			name:    "admin except own non deactivate",
			rule:    ruleAdminExceptSelfDeactivate,
			subject: admin,
			resource: enforcementResource{
				TargetUserID:  1,
				DisableTarget: false,
			},
			want: true,
		},
		{
			name:    "admin except self deactivate denied",
			rule:    ruleAdminExceptSelfDeactivate,
			subject: admin,
			resource: enforcementResource{
				TargetUserID:  1,
				DisableTarget: true,
			},
		},
		{
			name:    "owner",
			rule:    ruleOwnerOrAdmin,
			subject: member,
			resource: enforcementResource{
				OwnerUserID: 2,
			},
			want: true,
		},
		{
			name:    "owner admin",
			rule:    ruleOwnerOrAdmin,
			subject: admin,
			resource: enforcementResource{
				OwnerUserID: 2,
			},
			want: true,
		},
		{
			name:    "owner denied",
			rule:    ruleOwnerOrAdmin,
			subject: member,
			resource: enforcementResource{
				OwnerUserID: 1,
			},
		},
		{
			name:    "open non owner",
			rule:    ruleOpenNonOwnerOrAdmin,
			subject: member,
			resource: enforcementResource{
				OwnerUserID:    1,
				SubRequestOpen: true,
			},
			want: true,
		},
		{
			name:    "open owner admin",
			rule:    ruleOpenNonOwnerOrAdmin,
			subject: admin,
			resource: enforcementResource{
				OwnerUserID:    1,
				SubRequestOpen: true,
			},
			want: true,
		},
		{
			name:    "closed non owner denied",
			rule:    ruleOpenNonOwnerOrAdmin,
			subject: member,
			resource: enforcementResource{
				OwnerUserID:    1,
				SubRequestOpen: false,
			},
		},
		{
			name:    "taker",
			rule:    ruleTaker,
			subject: member,
			resource: enforcementResource{
				TakerUserID: 2,
				HasTaker:    true,
			},
			want: true,
		},
		{
			name:    "not taker denied",
			rule:    ruleTaker,
			subject: member,
			resource: enforcementResource{
				TakerUserID: 1,
				HasTaker:    true,
			},
		},
		{
			name:    "unknown denied",
			rule:    "unknown",
			subject: admin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesRule(tt.rule, tt.subject, tt.resource)
			if got != tt.want {
				t.Fatalf("matchesRule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegisterMatcherFunctionsValidatesArguments(t *testing.T) {
	admin := enforcementSubjectFrom(appauthz.SubjectFromCurrentUser(domain.CurrentUser{ID: 1, Role: domain.RoleAdmin}))
	resource := enforcementResource{Type: string(appauthz.ResourceAdmin)}

	tests := []struct {
		name    string
		args    []interface{}
		want    bool
		wantErr bool
	}{
		{
			name: "valid",
			args: []interface{}{
				ruleAdmin,
				admin,
				resource,
			},
			want: true,
		},
		{
			name:    "wrong count",
			args:    []interface{}{ruleAdmin, admin},
			wantErr: true,
		},
		{
			name:    "wrong rule type",
			args:    []interface{}{1, admin, resource},
			wantErr: true,
		},
		{
			name:    "wrong subject type",
			args:    []interface{}{ruleAdmin, "admin", resource},
			wantErr: true,
		},
		{
			name:    "wrong resource type",
			args:    []interface{}{ruleAdmin, admin, appauthz.AdminResource()},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := casbinMatchesRule(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("casbinMatchesRule error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Fatalf("casbinMatchesRule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthorizerSeedsPoliciesIntoSQLite(t *testing.T) {
	authorizer, db := newTestAuthorizer(t)
	defer db.Close()
	if authorizer == nil {
		t.Fatal("expected authorizer")
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM casbin_rule").Scan(&count); err != nil {
		t.Fatalf("count casbin policies: %v", err)
	}
	if count != len(baselinePolicies) {
		t.Fatalf("policy row count = %d, want %d", count, len(baselinePolicies))
	}

	again, err := NewAuthorizer(context.Background(), db)
	if err != nil {
		t.Fatalf("recreate authorizer: %v", err)
	}
	if again == nil {
		t.Fatal("expected recreated authorizer")
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM casbin_rule").Scan(&count); err != nil {
		t.Fatalf("count casbin policies after reseed: %v", err)
	}
	if count != len(baselinePolicies) {
		t.Fatalf("policy row count after reseed = %d, want %d", count, len(baselinePolicies))
	}
}

func TestNewAuthorizerValidatesDatabase(t *testing.T) {
	if _, err := NewAuthorizer(context.Background(), nil); err == nil {
		t.Fatal("expected nil database error")
	}
}

func TestNewAuthorizerReturnsLoadPolicyError(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.Close()

	if _, err := NewAuthorizer(context.Background(), db); err == nil {
		t.Fatal("expected closed database error")
	}
}

func TestSeedBaselinePolicyErrors(t *testing.T) {
	t.Run("load policy error", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		adapter, err := newSQLAdapter(context.Background(), db, sqliteDriverName, tableName)
		if err != nil {
			t.Fatalf("new sql adapter: %v", err)
		}
		model, err := newModelFromString(modelText)
		if err != nil {
			t.Fatalf("new model: %v", err)
		}
		enforcer, err := casbin.NewEnforcer(model, adapter)
		if err != nil {
			t.Fatalf("new enforcer: %v", err)
		}
		db.Close()

		if err := seedBaselinePolicy(context.Background(), enforcer); err == nil {
			t.Fatal("expected load policy error")
		}
	})

	t.Run("add policy error", func(t *testing.T) {
		authorizer, db := newTestAuthorizer(t)
		defer db.Close()
		if authorizer == nil {
			t.Fatal("expected authorizer")
		}
		originalAddBaselinePolicy := addBaselinePolicy
		addBaselinePolicy = func(enforcer *casbin.Enforcer) error {
			return errors.New("add failed")
		}
		t.Cleanup(func() { addBaselinePolicy = originalAddBaselinePolicy })

		if err := seedBaselinePolicy(context.Background(), authorizer.enforcer); err == nil {
			t.Fatal("expected add policy error")
		}
	})
}

func TestNewAuthorizerConstructorErrors(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	t.Run("model error", func(t *testing.T) {
		restoreAuthorizerConstructors(t)
		newModelFromString = func(text string) (casbinmodel.Model, error) {
			return nil, errors.New("model failed")
		}

		if _, err := NewAuthorizer(context.Background(), db); err == nil {
			t.Fatal("expected model error")
		}
	})

	t.Run("enforcer error", func(t *testing.T) {
		restoreAuthorizerConstructors(t)
		newEnforcer = func(params ...interface{}) (*casbin.Enforcer, error) {
			return nil, errors.New("enforcer failed")
		}

		if _, err := NewAuthorizer(context.Background(), db); err == nil {
			t.Fatal("expected enforcer error")
		}
	})

	t.Run("seed error", func(t *testing.T) {
		restoreAuthorizerConstructors(t)
		seedPolicy = func(ctx context.Context, enforcer *casbin.Enforcer) error {
			return errors.New("seed failed")
		}

		if _, err := NewAuthorizer(context.Background(), db); err == nil {
			t.Fatal("expected seed error")
		}
	})
}

func TestAuthorizerReturnsEnforceError(t *testing.T) {
	model, err := casbinmodel.NewModelFromString(`
[request_definition]
r = sub

[policy_definition]
p = sub

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = missingFunction(r.sub)
`)
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	enforcer, err := casbin.NewEnforcer(model)
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}
	authorizer := &Authorizer{enforcer: enforcer}

	err = authorizer.Authorize(context.Background(), appauthz.SubjectFromCurrentUser(domain.CurrentUser{Role: domain.RoleAdmin}), appauthz.ActionAdminAccess, appauthz.AdminResource())
	if err == nil {
		t.Fatal("expected enforce error")
	}
}

func newTestAuthorizer(t *testing.T) (*Authorizer, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	authorizer, err := NewAuthorizer(context.Background(), db)
	if err != nil {
		db.Close()
		t.Fatalf("new authorizer: %v", err)
	}
	return authorizer, db
}

func restoreAuthorizerConstructors(t *testing.T) {
	t.Helper()
	newSQLAdapter = sqladapter.NewAdapterWithContext
	newModelFromString = casbinmodel.NewModelFromString
	newEnforcer = casbin.NewEnforcer
	addBaselinePolicy = func(enforcer *casbin.Enforcer) error {
		_, err := enforcer.AddPoliciesEx(baselinePolicies)
		return err
	}
	seedPolicy = seedBaselinePolicy
	t.Cleanup(func() {
		newSQLAdapter = sqladapter.NewAdapterWithContext
		newModelFromString = casbinmodel.NewModelFromString
		newEnforcer = casbin.NewEnforcer
		addBaselinePolicy = func(enforcer *casbin.Enforcer) error {
			_, err := enforcer.AddPoliciesEx(baselinePolicies)
			return err
		}
		seedPolicy = seedBaselinePolicy
	})
}
