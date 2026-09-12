package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRegistrationGroupControllerTest provides a real (SQLite by default)
// database and the option tables the registration path reads from.
func setupRegistrationGroupControllerTest(t *testing.T) {
	t.Helper()
	db := setupManageUserTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Exec("DELETE FROM options").Error)
	require.NoError(t, authz.Init(db))

	originalRatios := ratio_setting.GroupRatio2JSONString()
	originalConfigured, _, originalErr := setting.RegistrationGroupConfiguration()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"free":0,"vip":2}`))
	require.NoError(t, setting.ApplyRegistrationGroupConfiguration("default", ratio_setting.GetGroupRatioCopy()))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
		if originalErr != nil {
			setting.ApplyInvalidRegistrationGroupConfiguration(originalConfigured, ratio_setting.GetGroupRatioCopy(), originalErr)
		} else {
			require.NoError(t, setting.ApplyRegistrationGroupConfiguration(originalConfigured, ratio_setting.GetGroupRatioCopy()))
		}
	})

	previousRegister, previousPasswordRegister := common.RegisterEnabled, common.PasswordRegisterEnabled
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	t.Cleanup(func() {
		common.RegisterEnabled, common.PasswordRegisterEnabled = previousRegister, previousPasswordRegister
	})
}

func performRegisterRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/register", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	Register(c)
	return recorder
}

func performCreateUserRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	c.Set(common.RequestIdKey, "registration-group-test")
	CreateUser(c)
	return recorder
}

func TestRegisterUsesConfiguredGroupAndIgnoresClientGroup(t *testing.T) {
	setupRegistrationGroupControllerTest(t)
	require.NoError(t, setting.ApplyRegistrationGroupConfiguration("free", ratio_setting.GetGroupRatioCopy()))

	recorder := performRegisterRequest(t, `{"username":"selfservice","password":"password-1234","group":"vip"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var created model.User
	require.NoError(t, model.DB.First(&created, "username = ?", "selfservice").Error)
	assert.Equal(t, "free", created.Group, "the server-configured group must win over the ignored client field")
}

func TestCreateUserKeepsDefaultGroup(t *testing.T) {
	setupRegistrationGroupControllerTest(t)
	require.NoError(t, setting.ApplyRegistrationGroupConfiguration("free", ratio_setting.GetGroupRatioCopy()))

	recorder := performCreateUserRequest(t, `{"username":"admin-created","password":"password-1234","role":1}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var created model.User
	require.NoError(t, model.DB.First(&created, "username = ?", "admin-created").Error)
	assert.Equal(t, "default", created.Group, "admin-created accounts must not use the self-registration group")
}

func TestExistingUserGroupIsNotMigratedWhenConfigurationChanges(t *testing.T) {
	setupRegistrationGroupControllerTest(t)
	existing := model.User{
		Username: "existing-member", Password: "password-1234", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "existing-aff",
	}
	require.NoError(t, model.DB.Create(&existing).Error)

	require.NoError(t, setting.ApplyRegistrationGroupConfiguration("free", ratio_setting.GetGroupRatioCopy()))

	var reloaded model.User
	require.NoError(t, model.DB.First(&reloaded, existing.Id).Error)
	assert.Equal(t, "default", reloaded.Group)
}

func TestRegisterFailsClosedWithoutLeakingGroupConfiguration(t *testing.T) {
	setupRegistrationGroupControllerTest(t)
	require.NoError(t, model.DB.Create(&model.Option{Key: "GroupRatio", Value: `{"default":1}`}).Error)
	setting.ApplyInvalidRegistrationGroupConfiguration("secret-group", map[string]float64{"default": 1}, nil)

	recorder := performRegisterRequest(t, `{"username":"blocked","password":"password-1234"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	message := i18n.T(nil, i18n.MsgUserRegisterFailed)
	assert.Contains(t, recorder.Body.String(), message)
	assert.NotContains(t, recorder.Body.String(), "secret-group")
	assert.NotContains(t, recorder.Body.String(), "registration group")

	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "blocked").Count(&count).Error)
	assert.Zero(t, count)
}
