package service

// 首个超管只能铸一次，而且这道判定必须待在 service 层。
//
// 立项依据是"公开入口的门只挂在一处"这件事迟早会漏：
// /api/system/init-admin 与 /api/system/create-default-admin 是公开路由（首次安装时没有登录态），
// 历史上"已初始化就拒绝"只写在 AuthController.InitAdmin 的开头一行。挂在入口上的守卫有两种失效方式，
// 本仓库都发生过：新增一个别名指向另一个 handler（SystemUserController.CreateDefaultAdmin 就是这条
// 旧入口，它当年没有那行判定），或者把守卫挪到参数校验之后（合法载荷就会真的建号）。
// 所以这里把判定沉到 InitAdmin 的第一句，用例断的是"无论谁来调都拦"，而不是"某条路由拦了"。
//
// 两道依据缺一不可，第 2 条补的是第 1 条的失效面：
//  1. install.lock 标记 initialized——正常安装后的状态。
//  2. 库里已有 role=admin 的账号——真相。lock 只是缓存：文件被删/写坏时，
//     install.GetStatus 在"数据库探针未装配"（返回空串，与"库里真没超管"同形）下会把实例读成
//     NOT_INSTALLED，只看第 1 条就给匿名建号留了一个窗口。
//
// 四条腿都不碰真库：既有超管数由桩回答，install.lock 由 INSTALL_LOCK_PATH 指进临时目录。

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/system/install"
)

// firstAdminProbeRepo 只实现 InitAdmin 会走到的那几个方法，其余方法留在嵌入的 nil 接口上：
// 一旦流程越界去调没实现的方法，测试会当场 panic 并报出方法名——这正是要的可归因性。
type firstAdminProbeRepo struct {
	repository.SystemUserRepository
	admins    int64
	countErr  error
	createErr error
	created   *model.SystemUser
}

func (r *firstAdminProbeRepo) CountAdmins(context.Context) (int64, error) {
	return r.admins, r.countErr
}

func (r *firstAdminProbeRepo) UsernameExists(context.Context, string, uint) (bool, error) {
	return false, nil
}

func (r *firstAdminProbeRepo) EmailExists(context.Context, string, uint) (bool, error) {
	return false, nil
}

func (r *firstAdminProbeRepo) Create(_ context.Context, user *model.SystemUser) error {
	r.created = user
	return r.createErr
}

// stopBeforeWrite 让 Create 在真的落库前返回错误，用来证明"走到了建号分支"。
var stopBeforeWrite = errors.New("测试在写库前停止")

// lockPathIn 返回一个临时目录下的 lock 路径；不写文件＝"这台机器还没有安装记录"。
func lockPathIn(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "install.lock")
}

// pinFreshLockDir 把 INSTALL_LOCK_PATH 指到临时目录里一份还不存在的 lock。
// 必须指：默认路径是相对的 ./install.lock，而 go test 的 CWD 就是本包目录。
func pinFreshLockDir(t *testing.T) string {
	t.Helper()
	path := lockPathIn(t)
	t.Setenv("INSTALL_LOCK_PATH", path)
	return path
}

// pinInitializedLock 落一份"已装完"的 lock 并把 INSTALL_LOCK_PATH 指过去。
// 顺序要紧：install.Save 每次现读生效路径，先设 env 才会写进这份临时文件，
// 否则会把"已初始化"落进进程 CWD（也就是本包源码目录）。
func pinInitializedLock(t *testing.T, adminUsername string) {
	t.Helper()
	pinFreshLockDir(t)
	lr := &install.Lock{
		InstallID:     "ins-test",
		AdminUsername: adminUsername,
		Initialized:   true,
		Version:       "test",
	}
	if err := install.Save(lr); err != nil {
		t.Fatalf("写测试 install.lock 失败: %v", err)
	}
	if !install.GetStatus().Initialized {
		t.Fatal("前置不成立：刚写的 initialized=true 应被读回来")
	}
}

// TestInitAdmin_RefusesWhenLockSaysInitialized lock 说装完了：字段完全合法的载荷也不许建号。
func TestInitAdmin_RefusesWhenLockSaysInitialized(t *testing.T) {
	pinInitializedLock(t, "admin")
	repo := &firstAdminProbeRepo{admins: 0}
	svc := &AuthService{systemUserRepo: repo}

	err := svc.InitAdmin(context.Background(), "zz_attacker", "Str0ng!Pass123", "a@b.co")
	if !errors.Is(err, ErrAdminAlreadyInitialized) {
		t.Fatalf("已初始化时应回 ErrAdminAlreadyInitialized，实际: %v", err)
	}
	// 文案要与控制层 403 透出的一句一致（response.ErrSystemAlreadyInitialized）
	if err.Error() != "系统已初始化，禁止重复创建超管" {
		t.Fatalf("错误文案要与 403 透出的一致，实际: %q", err.Error())
	}
	if repo.created != nil {
		t.Fatalf("响应说要拒绝，账号却建出来了: %+v", repo.created)
	}
}

// TestInitAdmin_RefusesWhenDBAlreadyHasAdmin 第 2 道依据独立开火的那一格：
// lock 没了（安装身份文件丢失），库里却明明坐着一个超管——旧代码在这一格会放行匿名建号。
func TestInitAdmin_RefusesWhenDBAlreadyHasAdmin(t *testing.T) {
	pinFreshLockDir(t)
	repo := &firstAdminProbeRepo{admins: 1}
	svc := &AuthService{systemUserRepo: repo}

	ctx := context.Background()
	if install.GetStatus().Initialized {
		t.Fatal("前置不成立：没有 lock 文件时安装态应为未初始化，否则本用例测不到库这一道依据")
	}
	if err := svc.InitAdmin(ctx, "zz_attacker", "Str0ng!Pass123", ""); !errors.Is(err, ErrAdminAlreadyInitialized) {
		t.Fatalf("库里已有超管时必须拒绝，实际: %v", err)
	}
	if repo.created != nil {
		t.Fatalf("响应说要拒绝，账号却建出来了: %+v", repo.created)
	}
}

// TestInitAdmin_RefusesWhenAdminCountUnknown 数不出既有超管＝无法证明"还是第一枚"，同样拒绝。
// 这一格是 fail-closed 的边界：查询失败不能被读成"库里干净"。
func TestInitAdmin_RefusesWhenAdminCountUnknown(t *testing.T) {
	pinFreshLockDir(t)
	unreachable := errors.New("dial tcp: connection refused")
	repo := &firstAdminProbeRepo{countErr: unreachable}
	svc := &AuthService{systemUserRepo: repo}

	ctx := context.Background()
	err := svc.InitAdmin(ctx, "admin", "Str0ng!Pass123", "")
	if err == nil {
		t.Fatal("数不到既有超管时应拒绝，不能放行建号")
	}
	if !errors.Is(err, unreachable) {
		t.Fatalf("应把查询失败原样带出以便定位，实际: %v", err)
	}
	if errors.Is(err, ErrAdminAlreadyInitialized) {
		t.Fatalf("失败原因是库不可达，不该伪装成已初始化: %v", err)
	}
	if repo.created != nil {
		t.Fatalf("计数都数不到就把账号写下去了: %+v", repo.created)
	}
}

// TestInitAdmin_AllowsFirstAdminOnCleanSlate 反向一格：全新实例必须还能装。
// 没有这一格，上面三格可以靠"任何请求一律拒绝"实现——那是把安装向导也锁死的另一种坏。
func TestInitAdmin_AllowsFirstAdminOnCleanSlate(t *testing.T) {
	pinFreshLockDir(t)
	repo := &firstAdminProbeRepo{admins: 0, createErr: stopBeforeWrite}
	svc := &AuthService{systemUserRepo: repo}

	ctx := context.Background()
	if err := svc.InitAdmin(ctx, "admin", "Str0ng!Pass123", ""); !errors.Is(err, stopBeforeWrite) {
		t.Fatalf("全新实例应一路走到建号（夹具在写库前停止），实际: %v", err)
	}
	if repo.created == nil {
		t.Fatal("前置不成立：夹具没收到 Create，后面那句断言是空的")
	}
	if repo.created.Role != model.SystemUserRoleAdmin {
		t.Fatalf("建出的账号应是超管，实际 role=%q", repo.created.Role)
	}
}
