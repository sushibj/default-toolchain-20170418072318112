package main

import (
	"fmt"
	"time"
	"github.com/openblockchain/obc-peer/openchain/chaincode/shim"
	"errors"
	"strings"
	"encoding/json"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"net/http"
	"net/url"
	"io/ioutil"
	"strconv"
	"reflect"
)

//==============================================================================================================================
//	 Structure Definitions 
//==============================================================================================================================
//	Chaincode - A blank struct for use with Shim (A HyperLedger included go file used for get/put state
//				and other HyperLedger functions)
//==============================================================================================================================
type  Chaincode struct {
}

type Event struct {
	Name            string `json:"name"`
	Time			string `json:"time"`
	Text			string `json:"text"`
	Obj_ID			string `json:"obj_id"`
	Users			[]string `json:"users"`
}

type EventHolder struct {
	Events 			[]Event `json:"events"`
}

//==============================================================================================================================
//	ECertResponse - Struct for storing the JSON response of retrieving an ECert. JSON OK -> Struct OK
//==============================================================================================================================
type ECertResponse struct {
	OK string `json:"OK"`
}	

const   ROLE_AUTHORITY  	=  0
const   ROLE_MANUFACTURER   =  1
const   ROLE_PRIVATE_ENTITY =  2
const   ROLE_LEASE_COMPANY  =  3
const   ROLE_SCRAP_MERCHANT =  4

//==============================================================================================================================
//	Init Function - Called when the user deploys the chaincode sets up base events (blank array)																
//==============================================================================================================================
func (t *Chaincode) init(stub *shim.ChaincodeStub) ([]byte, error) {

	var eh EventHolder
	
	bytes, err := json.Marshal(eh)
	
															if err != nil { return nil, errors.New("Error creating event record") }
																
	err = stub.PutState("Events", bytes)
	
															if err != nil { return nil, errors.New("Error creating blank event array") }

	return nil, nil
}

//==============================================================================================================================
//	 General Functions
//==============================================================================================================================
//	 get_ecert - Takes the name passed and calls out to the REST API for HyperLedger to retrieve the ecert
//				 for that user. Returns the ecert as retrived including html encoding.
//==============================================================================================================================
func (t *Chaincode) get_ecert(stub *shim.ChaincodeStub, name string) ([]byte, error) {
	
	var cert ECertResponse
	
	response, err := http.Get("http://localhost:5000/registrar/"+name+"/ecert") // Calls out to the HyperLedger REST API to get the ecert of the user with that name
    
															if err != nil { return nil, errors.New("Could not get ecert") }
	
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)			// Read the response from the http callout into the variable contents
	
															if err != nil { return nil, errors.New("Could not read body") }
	
	err = json.Unmarshal(contents, &cert)
	
															if err != nil { return nil, errors.New("ECert not found for user: "+name) }
															
	return []byte(string(cert.OK)), nil
}

//==============================================================================================================================
//	 check_role - Takes an ecert, decodes it to remove html encoding then parses it and checks the
// 				  certificates extensions containing the role before returning the role interger. Returns -1 if it errors
//==============================================================================================================================
func (t *Chaincode) check_role(stub *shim.ChaincodeStub, args []string) (int64, error) {																							
	ECertSubjectRole := asn1.ObjectIdentifier{2, 1, 3, 4, 5, 6, 7}																														
	
	decodedCert, err := url.QueryUnescape(args[0]);    		// make % etc normal //
	
															if err != nil { return -1, errors.New("Could not decode certificate") }
	
	pem, _ := pem.Decode([]byte(decodedCert))           	// Make Plain text   //

	x509Cert, err := x509.ParseCertificate(pem.Bytes);		// Extract Certificate from argument //
														
															if err != nil { return -1, errors.New("Couldn't parse certificate")	}

	var role int64
	for _, ext := range x509Cert.Extensions {				// Get Role out of Certificate and return it //
		if reflect.DeepEqual(ext.Id, ECertSubjectRole) {
			role, err = strconv.ParseInt(string(ext.Value), 10, len(ext.Value)*8)   
                                            			
															if err != nil { return -1, errors.New("Failed parsing role: " + err.Error())	}
			break
		}
	}
	
	return role, nil
}

//==============================================================================================================================
//	Event Functions
//==============================================================================================================================
//	Create Event - Creates a new event object using the data passed and the current time then appends it to the events array
//				   before saving the state to the ledger
//==============================================================================================================================
func (t *Chaincode) create_event(stub *shim.ChaincodeStub, event_name string, event_text string, event_obj_id string, event_users []string) ([]byte, error) {

	var e Event
	
	e.Name 	 = event_name
	e.Time	 = time.Now().Format("02/01/2006 15:04")
	e.Text	 = event_text
	e.Obj_ID = event_obj_id
	e.Users	 = event_users

	bytes, err := stub.GetState("Events")
		
															if err != nil { return nil, errors.New("Unable to get events") }
	
	var eh EventHolder
	
	err = json.Unmarshal(bytes, &eh)						
	
															if err != nil {	return nil, errors.New("Corrupt events record") }
															
	eh.Events = append(eh.Events, e)
	
	bytes, err = json.Marshal(eh)
	
															if err != nil { fmt.Print("Error creating event record") }

	err = stub.PutState("Events", bytes)

															if err != nil { return nil, errors.New("Unable to put the state") }

	return nil, nil
}

//==============================================================================================================================
//	get_events - Takes a users name and returns the events they are entitled to. If they are the regulator they see all events
//				 otherwise it calls a function to get the users events
//==============================================================================================================================
func (t *Chaincode) get_events(stub *shim.ChaincodeStub, args []string) ([]byte, error) {

	bytes, err := stub.GetState("Events")
		
																			if err != nil { return nil, errors.New("Unable to get events") }
																	
	var eh EventHolder
	
	err = json.Unmarshal(bytes, &eh)						
	
																			if err != nil {	return nil, errors.New("Corrupt events record") }
															
	ecert, err := t.get_ecert(stub, args[0])
	
																			if err != nil {	return nil, err }
																	
	role, err := t.check_role(stub,[]string{string(ecert)})
	
																			if err != nil { return nil, err }
																	
	if role == ROLE_AUTHORITY {												// Return all events if authority
			
			repNull := strings.Replace(string(bytes), "null", "[]", 1)		// If the array is blank it has the json value null so we need to make it an empty array
	
			return []byte(repNull), nil
	
	} else {
	
		return t.get_users_events(stub, eh, args[0])
		
	}
	
}

//==============================================================================================================================
//	get_obj_events - Takes an events array and returns all the events that occured between "from" and "to" in the array to the  
//				 	Object with ID obj_id
//==============================================================================================================================
func (t *Chaincode) get_obj_events(stub *shim.ChaincodeStub, events []Event, obj_id string, from int, to int, user string) []Event {
	
	var resp []Event
	
	for i := from; i < to; i++ {
		if events[i].Obj_ID == obj_id && !contains(events[i].Users, user) {
			resp = append(resp, events[i])
		}
	}
															
	return resp
}

//==============================================================================================================================
//	get_users_events - Takes a users name and loops through all events to find those with the user as a participant. When it
//				       finds an event it notes the ID of the object it refers to and then gets all the events from the last time 
//					   it found an event for that object to now where the user isn't involved as they have permission to view
//					   the objects history before they were involved
//==============================================================================================================================
func (t *Chaincode) get_users_events(stub *shim.ChaincodeStub, eh EventHolder, name string) ([]byte, error) {
	
	users_events := []Event{}
	var searched_to map[string]int
	searched_to = make(map[string]int)
	
	for i, event := range eh.Events {
		if contains(event.Users, name) {
			users_events = append(users_events, event)
			users_events = append(users_events, t.get_obj_events(stub, eh.Events, event.Obj_ID, searched_to[event.Obj_ID], i, name)...) // get the events of the car before the user had ownership
			searched_to[event.Obj_ID] = i;
		} 
	}
	
	var found_events EventHolder
	found_events.Events = users_events
	
	bytes, err := json.Marshal(found_events)
	
															if err != nil { fmt.Print("Error sending record") }
	
	repNull := strings.Replace(string(bytes), "null", "[]", 1)
	return []byte(repNull), nil
}
//==============================================================================================================================
//	 Router Functions
//==============================================================================================================================
//	Run - Called on chaincode invoke. Takes a function name passed and calls that function.
//==============================================================================================================================
func (t *Chaincode) Run(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	// Handle different functions
	if function == "init" { return t.init(stub) 
	} else if function == "create_event" {
		if(len(args) < 4) {
			return nil, errors.New("Invalid number of arguments supplied")
		}
		
		var users_involved []string
		
		for i := 3; i < len(args); i++ {
		
			users_involved = append(users_involved, args[i])
		}
		return t.create_event(stub, args[0], args[1], args[2], users_involved) 
	}
	
	return nil, errors.New("Function of that name not found")
}
//==============================================================================================================================
//	Query - Called on chaincode query. Takes a function name passed and calls that function.
//==============================================================================================================================
func (t *Chaincode) Query(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	
	if function == "get_events" { return t.get_events(stub, args) }
	
	return nil, errors.New("Function of that name not found")
}

//=================================================================================================================================
//	 Main - main - Starts up the chaincode
//=================================================================================================================================
func main() {

	err := shim.Start(new(Chaincode))
	
															if err != nil { fmt.Printf("Error starting Chaincode: %s", err) }
}

func contains(s []string, e string) bool {
    for _, a := range s {
        if a == e {
            return true
        }
    }
    return false
}