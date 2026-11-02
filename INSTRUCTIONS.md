# Way of working

This repository is read-only, click on "Use this template" => "Create a new repository" and work on your copy just as you would on a live one. \
Once you are satisfied with your work, please send us your repository url.

# Overview

This sample repo is pretty close to what we do at x company. \
The scope you own is found under `/ads`. \
You can also modify what is found under `/common`. \
Do not change any code under `/users`.

Consider the ad service to be user facing. \
You are a backend developer in a feature team composed of manager, product owner, web and mobile developers. \
Consider that ad writes are relatively sparse, but the ad reads endpoints are under heavy load (more read than write).

# Instructions

Here is the list of things to do:
1) There is a bug in the `GetAdByID` usecase, the usecase does not work, fix it.

2) Add a delete ad by id endpoint, usecase and dao method.

3) In the `GetAdByID` endpoint, add the username field based on the information found in the user service. Use the provided client in `users/pkg/client.go`.

4) Stakeholders want the field `price_cents` renamed to `price` in float in the API. What do you do?

5) Users are reporting that the bulk import is not working properly, it fails sometimes and when they try to run again, some ads appear twice, propose a fix.

6) A user can now publish at most 5 ads per rolling 24 hours through `POST /ads`, implement this quota. It does not apply to the bulk import.

The complexity is more or less increasing with each item, do not beat yourself up if you miss some things.

Finally, feel free to improve the code in any way that seem appropriate to you or fix any latent issue: the code you push should reflect what you feel comfortable shipping in production.
