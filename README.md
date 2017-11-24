## jenkinson

### Configuration

Before using jekinson, you need to tell it about your Jenkins credentials. You can do this by going to: http://JENKINS_HOST/user/JENKINS_USER/configure and copy the value from "Show Api Token"

The run jenkinson configure to introduce your credentials with:

    jenkinson configure

You can add different profiles using:

    jenkinson --profile ANOTHER_PROFILE configure

After running *configure* , jenkinson will check if [CSRF](https://wiki.jenkins.io/display/JENKINS/CSRF+Protection) is enabled in jenkins. If that's the case it will add jenkins' crumb parameter in order to send POST requests to jenkins.

