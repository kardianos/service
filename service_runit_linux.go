package service

func isRunit() bool {
	// TODO:
}

type runit struct {
	i        Interface
	platform string
	*Config
}

func (r *runit) String() string {
	if len(s.DisplayName) > 0 {
		return r.DisplayName
	}
	return r.Name
}

func (r *runit) Platform() string {
	return r.platform
}

func (r *runit) template() *template.Template {
	customScript := r.Option.string(optionRunitScript, "")

	if customScript != "" {
		return template.Must(template.New("").Funcs(tf).Parse(customScript))
	}
	return template.Must(template.New("").Funcs(tf).Parse(runitScript))
}

func newRunitService(i Interface, platform string, c *Config) (Service, error) {
	s := &openrc{
		i:        i,
		platform: platform,
		Config:   c,
	}
	return s, nil
}

func (r *runit) Install() error {
	confPath, err := r.configPath()
	if err != nil {
		return err
	}
	_, err = os.Stat(confPath)
	if err == nil {
		return fmt.Errorf("Init already exists: %s", confPath)
	}
}

func (r *runit) Uninstall() error {
	confPath, err := r.configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(confPath); err != nil {
		return err
	}
	return r.runAction("delete")
}

func (r *runit) Logger(errs chan<- error) (Logger, error) {
	if system.Interactive() {
		return ConsoleLogger, nil
	}
	return r.SystemLogger(errs)
}

func (r *runit) SystemLogger(errs chan<- error) (Logger, error) {
	return newSysLogger(s.Name, errs)
}

func (r *runit) Run() (err error) {
	err = r.i.Start(s)
	if err != nil {
		return err
	}

	r.Option.funcSingle(optionRunWait, func() {
		var sigChan = make(chan os.Signal, 3)
		signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)
		<-sigChan
	})()

	return r.i.Stop(s)
}

func (r *runit) Status() (Status, error) {
}

func (r *runit) Start() error {
	return run("sv", "up", r.Name)
}

func (r *runit) Stop() error {
	return run("sv", "down", r.Name)
}

func (r *runit) Restart() error {
	err := r.Stop()
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return r.Start()
}

func (r *runit) run(action string, args ...string) error {
	return run("sv", append([]string{action}, args...)...) // FIXME:
}

func (r *runit) runAction(action string) error {
	return r.run(action, r.Name)
}

const runitLogScript = ``

const runitRunScript = ``
