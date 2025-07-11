t --pvt-out tp,survey
sleep 2
t --mobile
echo waiting 40 seconds for a fix
sleep 40
t --fixed-pos-ecef 3978578.1754,-8652.1523,4968410.94136 --fixed-pos-acc 5
# this should do nothing: we are already static
t --static --pvt-out tp,survey
t --mobile
# this should start a survey
t --survey --pvt-out tp,survey
sleep 3
# this should not start a survey
t --static --survey-acc 10 --survey-time 5000
sleep 3
# this should start a survey and force resurvey
t --survey --survey-acc 10 --survey-time 5000
sleep 3
# this should start a survey and force a resurvey
t --survey
sleep 3
# this should turn off survey messages
t --mobile --pvt-out tp,survey,off

