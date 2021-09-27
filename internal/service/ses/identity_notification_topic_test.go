package ses_test

import (
	"fmt"
	"log"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/hashicorp/terraform-provider-aws/internal/acctest"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/provider"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
)

func TestAccAwsSESIdentityNotificationTopic_basic(t *testing.T) {
	domain := acctest.RandomDomainName()
	topicName := sdkacctest.RandomWithPrefix("test-topic")
	resourceName := "aws_ses_identity_notification_topic.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			testAccPreCheckAWSSES(t)
		},
		ErrorCheck:   acctest.ErrorCheck(t, ses.EndpointsID),
		Providers:    acctest.Providers,
		CheckDestroy: testAccCheckAwsSESIdentityNotificationTopicDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(testAccAwsSESIdentityNotificationTopicConfig_basic, domain),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAwsSESIdentityNotificationTopicExists(resourceName),
				),
			},
			{
				Config: fmt.Sprintf(testAccAwsSESIdentityNotificationTopicConfig_update, domain, topicName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAwsSESIdentityNotificationTopicExists(resourceName),
				),
			},
			{
				Config: fmt.Sprintf(testAccAwsSESIdentityNotificationTopicConfig_headers, domain, topicName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAwsSESIdentityNotificationTopicExists(resourceName),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckAwsSESIdentityNotificationTopicDestroy(s *terraform.State) error {
	conn := acctest.Provider.Meta().(*conns.AWSClient).SESConn

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "aws_ses_identity_notification_topic" {
			continue
		}

		identity := rs.Primary.Attributes["identity"]
		params := &ses.GetIdentityNotificationAttributesInput{
			Identities: []*string{aws.String(identity)},
		}

		log.Printf("[DEBUG] Testing SES Identity Notification Topic Destroy: %#v", params)

		response, err := conn.GetIdentityNotificationAttributes(params)
		if err != nil {
			return err
		}

		if response.NotificationAttributes[identity] != nil {
			return fmt.Errorf("SES Identity Notification Topic %s still exists. Failing!", identity)
		}
	}

	return nil
}

func testAccCheckAwsSESIdentityNotificationTopicExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("SES Identity Notification Topic not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("SES Identity Notification Topic identity not set")
		}

		identity := rs.Primary.Attributes["identity"]
		conn := acctest.Provider.Meta().(*conns.AWSClient).SESConn

		params := &ses.GetIdentityNotificationAttributesInput{
			Identities: []*string{aws.String(identity)},
		}

		log.Printf("[DEBUG] Testing SES Identity Notification Topic Exists: %#v", params)

		response, err := conn.GetIdentityNotificationAttributes(params)
		if err != nil {
			return err
		}

		if response.NotificationAttributes[identity] == nil {
			return fmt.Errorf("SES Identity Notification Topic %s not found in AWS", identity)
		}

		notificationType := rs.Primary.Attributes["notification_type"]
		headersExpected, _ := strconv.ParseBool(rs.Primary.Attributes["include_original_headers"])

		var headersIncluded bool
		switch notificationType {
		case ses.NotificationTypeBounce:
			headersIncluded = *response.NotificationAttributes[identity].HeadersInBounceNotificationsEnabled
		case ses.NotificationTypeComplaint:
			headersIncluded = *response.NotificationAttributes[identity].HeadersInComplaintNotificationsEnabled
		case ses.NotificationTypeDelivery:
			headersIncluded = *response.NotificationAttributes[identity].HeadersInDeliveryNotificationsEnabled
		}

		if headersIncluded != headersExpected {
			return fmt.Errorf("Wrong value applied for include_original_headers for %s", identity)
		}

		return nil
	}
}

const testAccAwsSESIdentityNotificationTopicConfig_basic = `
resource "aws_ses_identity_notification_topic" "test" {
  identity          = aws_ses_domain_identity.test.arn
  notification_type = "Complaint"
}

resource "aws_ses_domain_identity" "test" {
  domain = "%s"
}
`

const testAccAwsSESIdentityNotificationTopicConfig_update = `
resource "aws_ses_identity_notification_topic" "test" {
  topic_arn         = aws_sns_topic.test.arn
  identity          = aws_ses_domain_identity.test.arn
  notification_type = "Complaint"
}

resource "aws_ses_domain_identity" "test" {
  domain = "%s"
}

resource "aws_sns_topic" "test" {
  name = "%s"
}
`

const testAccAwsSESIdentityNotificationTopicConfig_headers = `
resource "aws_ses_identity_notification_topic" "test" {
  topic_arn                = aws_sns_topic.test.arn
  identity                 = aws_ses_domain_identity.test.arn
  notification_type        = "Complaint"
  include_original_headers = true
}

resource "aws_ses_domain_identity" "test" {
  domain = "%s"
}

resource "aws_sns_topic" "test" {
  name = "%s"
}
`