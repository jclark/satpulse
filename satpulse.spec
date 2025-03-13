Name: satpulse
Version: %{version}
Release: 1
Summary: Use a GPS receiver to provide precision time to PTP and NTP daemons
License: MIT
URL: https://github.com/jclark/satpulse

%global _build_id_links none
%global source_date_epoch_from_changelog 0
%global __os_install_post %{nil}
%global selinuxtype targeted

%description
This requires an ethernet controller with a PPS input pin that is supported by
the kernel's PTP hardware clock infrastructure, such as the Intel i210
or the Raspberry Pi CM4.

%install
rm -rf %{buildroot}
install -D out/%{goarch}/satpulsed %{buildroot}/usr/sbin/satpulsed
install -D out/%{goarch}/satpulsetool %{buildroot}/usr/bin/satpulsetool
install -D -m 644 out/%{goarch}/satpulse.toml %{buildroot}/etc/satpulse.toml
install -D -m 644 configs/satpulse@.service %{buildroot}/usr/lib/systemd/system/satpulse@.service
install -D -m 644 configs/chrony.conf %{buildroot}/usr/share/doc/satpulse/chrony.conf
install -D -m 644 configs/config-schema.json %{buildroot}/usr/share/doc/satpulse/config-schema.json
install -D -m 644 docs/config.md %{buildroot}/usr/share/doc/satpulse/config.md
install -D -m 644 docs/quickstart.md %{buildroot}/usr/share/doc/satpulse/quickstart.md
install -D -m 644 LICENSE %{buildroot}/usr/share/doc/satpulse/copyright
install -D -m 644 selinux/satpulse.pp.bz2 %{buildroot}/usr/share/selinux/packages/%{selinuxtype}/satpulse.pp.bz2

%post
systemctl daemon-reload
semodule -i /usr/share/selinux/packages/%{selinuxtype}/satpulse.pp.bz2 >/dev/null 2>&1 || :

%postun
if [ $1 -eq 0 ]; then
    semodule -r satpulse >/dev/null 2>&1 || :
fi

%files
/usr/sbin/satpulsed
/usr/bin/satpulsetool
%config(noreplace) /etc/satpulse.toml
/usr/lib/systemd/system/satpulse@.service
/usr/share/doc/satpulse/chrony.conf
/usr/share/doc/satpulse/config-schema.json
/usr/share/doc/satpulse/config.md
/usr/share/doc/satpulse/quickstart.md
/usr/share/doc/satpulse/copyright
/usr/share/selinux/packages/%{selinuxtype}/satpulse.pp.bz2
