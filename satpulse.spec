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
install -D out/%{goarch}/satpulsewb %{buildroot}/usr/bin/satpulsewb
install -D -m 644 out/%{goarch}/satpulse.toml %{buildroot}/etc/satpulse.toml
install -D -m 644 configs/satpulse@.service %{buildroot}/usr/lib/systemd/system/satpulse@.service
install -D -m 644 configs/chrony.conf %{buildroot}/usr/share/doc/satpulse/chrony.conf
install -D -m 644 configs/config-schema.json %{buildroot}/usr/share/doc/satpulse/config-schema.json
for f in `cat out/gpsmsg.files`; do
    install -D -m 644 configs/gpsmsg/$f %{buildroot}/usr/share/satpulse/gpsmsg/$f
done
install -D -m 644 LICENSE %{buildroot}/usr/share/doc/satpulse/copyright
install -D -m 644 selinux/satpulse.pp.bz2 %{buildroot}/usr/share/selinux/packages/%{selinuxtype}/satpulse.pp.bz2
install -D -m 644 out/satpulsetool.1.gz %{buildroot}/usr/share/man/man1/satpulsetool.1.gz
install -D -m 644 out/satpulsetool-gps.1.gz %{buildroot}/usr/share/man/man1/satpulsetool-gps.1.gz
install -D -m 644 out/satpulsetool-pack.1.gz %{buildroot}/usr/share/man/man1/satpulsetool-pack.1.gz
install -D -m 644 out/satpulsetool-scan.1.gz %{buildroot}/usr/share/man/man1/satpulsetool-scan.1.gz
install -D -m 644 out/satpulsetool-sdp.1.gz %{buildroot}/usr/share/man/man1/satpulsetool-sdp.1.gz
install -D -m 644 out/satpulsetool-syncsim.1.gz %{buildroot}/usr/share/man/man1/satpulsetool-syncsim.1.gz
install -D -m 644 out/satpulsetool-convobs.1.gz %{buildroot}/usr/share/man/man1/satpulsetool-convobs.1.gz
install -D -m 644 out/satpulsewb.1.gz %{buildroot}/usr/share/man/man1/satpulsewb.1.gz
install -D -m 644 out/satpulse.toml.5.gz %{buildroot}/usr/share/man/man5/satpulse.toml.5.gz
install -D -m 644 out/satpulsed.8.gz %{buildroot}/usr/share/man/man8/satpulsed.8.gz

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
/usr/bin/satpulsewb
%config(noreplace) /etc/satpulse.toml
/usr/lib/systemd/system/satpulse@.service
/usr/share/doc/satpulse/chrony.conf
/usr/share/doc/satpulse/config-schema.json
%dir /usr/share/satpulse
/usr/share/satpulse/gpsmsg
/usr/share/doc/satpulse/copyright
/usr/share/selinux/packages/%{selinuxtype}/satpulse.pp.bz2
/usr/share/man/man1/satpulsetool.1.gz
/usr/share/man/man1/satpulsetool-gps.1.gz
/usr/share/man/man1/satpulsetool-pack.1.gz
/usr/share/man/man1/satpulsetool-scan.1.gz
/usr/share/man/man1/satpulsetool-sdp.1.gz
/usr/share/man/man1/satpulsetool-syncsim.1.gz
/usr/share/man/man1/satpulsetool-convobs.1.gz
/usr/share/man/man1/satpulsewb.1.gz
/usr/share/man/man5/satpulse.toml.5.gz
/usr/share/man/man8/satpulsed.8.gz
